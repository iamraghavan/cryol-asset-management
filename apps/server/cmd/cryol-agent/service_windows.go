package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"github.com/cryol/cryol-asset-management/apps/server/internal/inventory"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	serviceName        = "CryolAgent"
	serviceDisplayName = "Cryol Endpoint Inventory"
	serviceDescription = "Collects authorized hardware, installed-software and network inventory and reports it to your organization's Cryol server."
)

func platformCommand(ctx context.Context, log *slog.Logger, args []string) (bool, error) {
	switch args[0] {
	case "install-service":
		return true, installService(ctx, log, args[1:])
	case "status-service":
		if len(args) != 1 {
			return true, errors.New("usage: cryol-agent status-service")
		}
		return true, installedServiceStatus()
	case "repair-service":
		if len(args) != 1 {
			return true, errors.New("usage: cryol-agent repair-service")
		}
		return true, repairService(ctx)
	case "uninstall-service":
		if len(args) != 1 {
			return true, errors.New("usage: cryol-agent uninstall-service")
		}
		return true, uninstallService(ctx)
	case "service":
		return true, svc.Run(serviceName, inventoryService{args: append([]string{"run"}, args[1:]...)})
	default:
		return false, nil
	}
}

func lockAgent(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	var over windows.Overlapped
	h := windows.Handle(f.Fd())
	if err := windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &over); err != nil {
		f.Close()
		return nil, err
	}
	return func() { _ = f.Close() }, nil
}

func secureDirectory(path, descriptor string) error {
	sd, err := windows.SecurityDescriptorFromString(descriptor)
	if err != nil {
		return err
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	return windows.CreateDirectory(name, &sa)
}

func copySetupFile(source, target string, max int64) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > max {
		return errors.New("setup file is not a supported regular file")
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(out, io.LimitReader(in, max+1))
	closeErr := out.Close()
	if n > max {
		return errors.New("setup file exceeds size limit")
	}
	return errors.Join(copyErr, closeErr)
}

func installService(ctx context.Context, log *slog.Logger, args []string) (result error) {
	fs := flag.NewFlagSet("install-service", flag.ContinueOnError)
	server := fs.String("server", "", "Cryol HTTPS server origin")
	ca := fs.String("ca", "", "optional organization CA certificate")
	interval := fs.Duration("interval", 5*time.Minute, "inventory scan interval")
	local := fs.Bool("local-http", false, "allow an explicit same-computer development connection")
	showToken := fs.Bool("show-token", false, "show enrollment input while typing or pasting")
	installerInput := fs.Bool("installer-input", false, "accept the enrollment token from the elevated setup process")
	archiveExisting := fs.Bool("archive-existing", false, "archive a leftover local Cryol directory before a fresh enrollment")
	pause := fs.Bool("pause", false, "keep the elevated setup window open after installation")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var token []byte
	if *installerInput {
		if !windows.GetCurrentProcessToken().IsElevated() {
			return errors.New("the setup wizard must be elevated")
		}
		*server = os.Getenv("CRYOL_SETUP_SERVER")
		*ca = os.Getenv("CRYOL_SETUP_CA")
		rawInterval := os.Getenv("CRYOL_SETUP_INTERVAL")
		token = []byte(os.Getenv("CRYOL_SETUP_ENROLLMENT_TOKEN"))
		defer clear(token)
		for _, key := range []string{"CRYOL_SETUP_SERVER", "CRYOL_SETUP_CA", "CRYOL_SETUP_INTERVAL", "CRYOL_SETUP_ENROLLMENT_TOKEN"} {
			if err := os.Unsetenv(key); err != nil {
				return err
			}
		}
		parsed, err := time.ParseDuration(rawInterval)
		if err != nil || parsed < time.Minute {
			return errors.New("enter a scan interval of at least one minute, for example 5m")
		}
		*interval = parsed
		if !validEnrollmentToken(token) {
			return errors.New("enter a valid enrollment token from the Cryol console")
		}
		*local = false
	}
	if fs.NArg() != 0 || *server == "" || *interval < time.Minute {
		return errors.New("provide a server address and a scan interval of at least one minute")
	}
	if !windows.GetCurrentProcessToken().IsElevated() {
		exec, err := os.Executable()
		if err != nil {
			return err
		}
		file, err := windows.UTF16PtrFromString(exec)
		if err != nil {
			return err
		}
		parameters, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(append(append([]string{"install-service"}, args...), "-pause")))
		if err != nil {
			return err
		}
		verb, _ := windows.UTF16PtrFromString("runas")
		return windows.ShellExecute(0, verb, file, parameters, nil, 1)
	}
	if *pause {
		defer func() {
			if result != nil {
				fmt.Fprintln(os.Stderr, result)
			}
			fmt.Fprint(os.Stderr, "Press Enter to close setup. ")
			_, _ = fmt.Fscanln(os.Stdin)
		}()
	}
	programs, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return err
	}
	unlock, err := lockAgent(filepath.Join(programs, "Cryol Agent Setup.lock"))
	if err != nil {
		return fmt.Errorf("another Cryol setup may be active: %w", err)
	}
	defer unlock()
	root := filepath.Join(programs, "Cryol Agent")
	state := filepath.Join(root, "state")
	executable := filepath.Join(root, "cryol-agent.exe")
	source, err := os.Executable()
	if err != nil {
		return err
	}
	manager, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer manager.Disconnect()
	if service, err := manager.OpenService(serviceName); err == nil {
		defer service.Close()
		return repairInstalledService(ctx, service, root, source)
	} else if !errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return err
	}

	backup, err := archivePreviousInstall(root, *archiveExisting)
	if err != nil {
		return err
	}
	if backup != "" {
		fmt.Println("Previous Cryol files were archived at", backup)
		defer func() {
			if result != nil {
				result = fmt.Errorf("%w. Previous files are preserved at %s", result, backup)
			}
		}()
	}
	if err := secureDirectory(root, `O:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;GRGX;;;LS)`); err != nil {
		return fmt.Errorf("create protected installation directory: %w", err)
	}
	retain := false
	defer func() {
		if !retain {
			_ = os.RemoveAll(root)
		}
	}()
	// LocalService can write state, but cannot replace the installed program.
	if err := secureDirectory(state, `O:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;0x1301bf;;;LS)`); err != nil {
		return err
	}
	if err := copySetupFile(source, executable, 64<<20); err != nil {
		return err
	}
	runArgs := []string{"-server", *server, "-state-dir", state, "-interval", interval.String()}
	if *local {
		runArgs = append(runArgs, "-local-http")
	}
	if *ca != "" {
		cert := filepath.Join(root, "cryol-ca.pem")
		if err := copySetupFile(*ca, cert, 2<<20); err != nil {
			return err
		}
		runArgs = append(runArgs, "-ca", cert)
	}
	service, err := manager.CreateService(serviceName, executable, mgr.Config{
		DisplayName:      serviceDisplayName,
		Description:      serviceDescription,
		StartType:        mgr.StartManual,
		ServiceStartName: `NT AUTHORITY\LocalService`,
	}, append([]string{"service"}, runArgs...)...)
	if err != nil {
		return err
	}
	defer service.Close()
	defer func() {
		if !retain {
			_ = service.Delete()
		}
	}()
	if err := configureServiceRecovery(service); err != nil {
		return err
	}
	createdSource, err := ensureServiceEventSource()
	if err != nil {
		return fmt.Errorf("register Windows event log source: %w", err)
	}
	defer func() {
		if !retain && createdSource {
			_ = eventlog.Remove(serviceName)
		}
	}()
	enrollArgs := append([]string{"enroll"}, runArgs...)
	if *showToken {
		enrollArgs = append(enrollArgs, "-show-token")
	}
	if err := runInventoryWithEnrollment(ctx, log, enrollArgs, token); err != nil {
		return err
	}

	if err := configureServiceStartup(service); err != nil {
		retain = true
		return fmt.Errorf("agent enrolled, but Windows service settings could not be verified: %w", err)
	}
	// Keep the enrolled identity if Windows cannot start the service immediately.
	retain = true
	return startAndVerifyInventory(ctx, service, state)
}

func archivePreviousInstall(root string, allowed bool) (string, error) {
	path, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return "", err
	}
	attributes, err := windows.GetFileAttributes(path)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("check existing Cryol directory: %w", err)
	}
	if attributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return "", errors.New("the Cryol installation path is a file or a redirected directory. Setup left it unchanged")
	}
	if !allowed {
		return "", errors.New("the Cryol directory already exists. If the service is no longer registered, reopen Setup and choose Archive previous local files and enroll")
	}

	// Archive on the same volume so existing filesystem permissions and data are preserved.
	backup := root + " Backup " + time.Now().Format("20060102-150405") + " " + inventory.NewID()[:8]
	target, err := windows.UTF16PtrFromString(backup)
	if err != nil {
		return "", err
	}
	if err := windows.MoveFile(path, target); err != nil {
		return "", fmt.Errorf("could not archive previous Cryol files: %w", err)
	}
	return backup, nil
}

func ensureServiceEventSource() (bool, error) {
	const path = `SYSTEM\CurrentControlSet\Services\EventLog\Application\CryolAgent`
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return true, eventlog.InstallAsEventCreate(serviceName, eventlog.Error|eventlog.Warning|eventlog.Info)
	}
	if err != nil {
		return false, err
	}
	defer key.Close()
	messageFile, _, err := key.GetStringValue("EventMessageFile")
	if err != nil || !strings.EqualFold(messageFile, `%SystemRoot%\System32\EventCreate.exe`) {
		return false, errors.New("an unexpected CryolAgent Event Viewer configuration exists")
	}
	return false, nil
}

func configureServiceStartup(service *mgr.Service) error {
	display, err := windows.UTF16PtrFromString(serviceDisplayName)
	if err != nil {
		return err
	}
	// Preserve the installed command line, account, dependencies and credentials.
	if err := windows.ChangeServiceConfig(service.Handle, windows.SERVICE_NO_CHANGE, windows.SERVICE_AUTO_START,
		windows.SERVICE_NO_CHANGE, nil, nil, nil, nil, nil, nil, display); err != nil {
		return err
	}
	details, err := windows.UTF16PtrFromString(serviceDescription)
	if err != nil {
		return err
	}
	description := windows.SERVICE_DESCRIPTION{Description: details}
	if err := windows.ChangeServiceConfig2(service.Handle, windows.SERVICE_CONFIG_DESCRIPTION, (*byte)(unsafe.Pointer(&description))); err != nil {
		return fmt.Errorf("restore service description: %w", err)
	}
	var delayed windows.SERVICE_DELAYED_AUTO_START_INFO
	if err := windows.ChangeServiceConfig2(service.Handle, windows.SERVICE_CONFIG_DELAYED_AUTO_START_INFO, (*byte)(unsafe.Pointer(&delayed))); err != nil {
		return fmt.Errorf("set automatic startup: %w", err)
	}
	readback, err := service.Config()
	if err != nil {
		return err
	}
	if readback.Description != serviceDescription || readback.DisplayName != serviceDisplayName || readback.StartType != mgr.StartAutomatic || readback.DelayedAutoStart {
		return errors.New("Windows did not report the expected service description and automatic startup")
	}
	return nil
}

func configureServiceRecovery(service *mgr.Service) error {
	return service.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 2 * time.Minute},
		{Type: mgr.NoAction},
	}, 86400)
}

// Check the core service identity without reading its potentially damaged description.
func readServiceIdentity(handle windows.Handle) (command, account string, serviceType uint32, err error) {
	var needed uint32
	err = windows.QueryServiceConfig(handle, nil, 0, &needed)
	if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) {
		if err == nil {
			err = errors.New("Windows did not report a configuration size")
		}
		return "", "", 0, err
	}
	if needed < uint32(unsafe.Sizeof(windows.QUERY_SERVICE_CONFIG{})) || needed > 64<<10 {
		return "", "", 0, errors.New("Windows reported an invalid service configuration size")
	}
	buffer := make([]byte, needed)
	config := (*windows.QUERY_SERVICE_CONFIG)(unsafe.Pointer(&buffer[0]))
	if err := windows.QueryServiceConfig(handle, config, needed, &needed); err != nil {
		return "", "", 0, err
	}
	return windows.UTF16PtrToString(config.BinaryPathName), windows.UTF16PtrToString(config.ServiceStartName), config.ServiceType, nil
}

func installedServiceStatus() error {
	manager, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer manager.Disconnect()
	service, err := manager.OpenService(serviceName)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		fmt.Println("Cryol Endpoint Inventory is not registered.")
		return nil
	}
	if err != nil {
		return err
	}
	defer service.Close()
	status, err := service.Query()
	if err != nil {
		return err
	}
	states := map[svc.State]string{svc.Running: "Running", svc.Stopped: "Stopped", svc.StartPending: "Starting", svc.StopPending: "Stopping", svc.Paused: "Paused"}
	name := states[status.State]
	if name == "" {
		name = fmt.Sprintf("State %d", status.State)
	}
	fmt.Printf("Cryol Endpoint Inventory: %s\r\nWindows exit code: %d\r\n", name, status.Win32ExitCode)
	if cfg, err := service.Config(); err == nil {
		if cfg.StartType == mgr.StartAutomatic {
			fmt.Println("Startup: Automatic")
		} else {
			fmt.Println("Startup: Not automatic")
		}
		if cfg.Description == "" {
			fmt.Println("Service description is missing.")
		}
	} else {
		fmt.Println("Windows could not read all service settings.")
	}
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return err
	}
	for _, item := range []struct{ name, path string }{
		{"Installed executable", filepath.Join(root, "Cryol Agent", "cryol-agent.exe")},
		{"Enrollment file", filepath.Join(root, "Cryol Agent", "state", "identity.json")},
	} {
		label := "missing or unreadable"
		if info, err := os.Lstat(item.path); err == nil && info.Mode().IsRegular() {
			label = "present"
		}
		fmt.Printf("%s: %s\r\n", item.name, label)
	}
	return nil
}

func repairService(ctx context.Context) error {
	if !windows.GetCurrentProcessToken().IsElevated() {
		return errors.New("run the Cryol setup wizard as an administrator")
	}
	programs, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return err
	}
	unlock, err := lockAgent(filepath.Join(programs, "Cryol Agent Setup.lock"))
	if err != nil {
		return fmt.Errorf("another Cryol setup may be active: %w", err)
	}
	defer unlock()
	root := filepath.Join(programs, "Cryol Agent")
	manager, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer manager.Disconnect()
	service, err := manager.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("open CryolAgent for repair: %w", err)
	}
	defer service.Close()
	source, err := os.Executable()
	if err != nil {
		return err
	}
	return repairInstalledService(ctx, service, root, source)
}

func repairInstalledService(ctx context.Context, service *mgr.Service, root, source string) error {
	expected := filepath.Join(root, "cryol-agent.exe")
	stateDir := filepath.Join(root, "state")
	commandLine, account, serviceType, err := readServiceIdentity(service.Handle)
	if err != nil {
		return fmt.Errorf("read CryolAgent configuration: %w", err)
	}
	command, err := windows.DecomposeCommandLine(commandLine)
	if err != nil || len(command) < 2 || !strings.EqualFold(filepath.Clean(command[0]), expected) || command[1] != "service" ||
		!strings.EqualFold(account, `NT AUTHORITY\LocalService`) || serviceType != windows.SERVICE_WIN32_OWN_PROCESS {
		return errors.New("this service does not match the expected Cryol installation and was left unchanged")
	}
	for _, dir := range []string{root, stateDir} {
		path, err := windows.UTF16PtrFromString(dir)
		if err != nil {
			return err
		}
		attributes, err := windows.GetFileAttributes(path)
		if err != nil || attributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return fmt.Errorf("Cryol repair requires a local installation directory: %s", dir)
		}
	}
	if info, err := os.Lstat(filepath.Join(stateDir, "identity.json")); err != nil || !info.Mode().IsRegular() {
		return errors.New("the local device credential is missing. Unregister the broken service and use Setup's archive and enroll option with a fresh token")
	}
	if _, err := ensureServiceEventSource(); err != nil {
		return fmt.Errorf("restore Windows event log source: %w", err)
	}
	if err := configureServiceStartup(service); err != nil {
		return err
	}
	if err := configureServiceRecovery(service); err != nil {
		return fmt.Errorf("update service recovery: %w", err)
	}
	status, err := service.Query()
	if err != nil {
		return err
	}
	if status.State == svc.StartPending {
		if err := waitForService(ctx, service, svc.Running); err != nil {
			return err
		}
		status.State = svc.Running
	}
	if status.State != svc.Stopped {
		if status.State != svc.StopPending {
			if _, err := service.Control(svc.Stop); err != nil && !errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) {
				return fmt.Errorf("stop CryolAgent for repair: %w", err)
			}
		}
		if err := waitForService(ctx, service, svc.Stopped); err != nil {
			return err
		}
	}
	if !strings.EqualFold(filepath.Clean(source), expected) {
		// Stage beside the installed executable, then replace it on the same volume.
		staged := expected + ".setup-" + inventory.NewID()
		defer os.Remove(staged)
		if err := copySetupFile(source, staged, 64<<20); err != nil {
			_ = service.Start()
			return fmt.Errorf("stage the Cryol agent update: %w", err)
		}
		if err := os.Rename(staged, expected); err != nil {
			_ = service.Start()
			return fmt.Errorf("replace the Cryol agent executable: %w", err)
		}
	}
	return startAndVerifyInventory(ctx, service, stateDir)
}

func startAndVerifyInventory(ctx context.Context, service *mgr.Service, stateDir string) error {
	receipt := filepath.Join(stateDir, inventoryReceipt)
	if err := os.Remove(receipt); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("reset the Cryol upload acknowledgement: %w", err)
	}
	if err := service.Start(); err != nil && !errors.Is(err, windows.ERROR_SERVICE_ALREADY_RUNNING) {
		return fmt.Errorf("Windows could not start CryolAgent: %w", err)
	}
	if err := waitForService(ctx, service, svc.Running); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		if info, err := os.Lstat(receipt); err == nil && info.Mode().IsRegular() {
			fmt.Println("Cryol Endpoint Inventory is running and the server accepted an inventory report.")
			return nil
		}
		status, err := service.Query()
		if err != nil {
			return err
		}
		if status.State == svc.Stopped {
			return errors.New("CryolAgent stopped before uploading inventory. Check the CryolAgent source in Windows Event Viewer for credential, certificate or collector errors")
		}
		select {
		case <-ctx.Done():
			return errors.New("CryolAgent started, but the server has not confirmed an inventory upload. The service remains installed. Check the server address and the CryolAgent source in Windows Event Viewer")
		case <-ticker.C:
		}
	}
}

type inventoryService struct{ args []string }

func (s inventoryService) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	event, err := eventlog.Open(serviceName)
	if err != nil {
		return true, 1
	}
	defer event.Close()
	log := slog.New(slog.NewJSONHandler(serviceLog{event}, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runInventory(ctx, log, s.args) }()
	running := svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	status <- running
	for {
		select {
		case request := <-requests:
			switch request.Cmd {
			case svc.Interrogate:
				status <- running
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending, WaitHint: 10000}
				cancel()
				select {
				case <-done:
					return false, 0
				case <-time.After(10 * time.Second):
					return true, 1
				}
			}
		case err := <-done:
			if err != nil {
				log.Error("inventory service stopped", "error", err)
			}
			return true, 1
		}
	}
}

type serviceLog struct{ event *eventlog.Log }

func (s serviceLog) Write(data []byte) (int, error) {
	message := strings.TrimSpace(string(data))
	switch {
	case strings.Contains(message, `"level":"ERROR"`):
		return len(data), s.event.Error(1, message)
	case strings.Contains(message, `"level":"WARN"`):
		return len(data), s.event.Warning(1, message)
	default:
		return len(data), s.event.Info(1, message)
	}
}

func waitForService(ctx context.Context, service *mgr.Service, wanted svc.State) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	timer := time.NewTicker(250 * time.Millisecond)
	defer timer.Stop()
	for {
		state, err := service.Query()
		if err != nil {
			return err
		}
		if state.State == wanted {
			return nil
		}
		if wanted == svc.Running && state.State == svc.Stopped {
			return errors.New("agent service stopped during startup; check Windows Event Viewer")
		}
		select {
		case <-timer.C:
		case <-ctx.Done():
			return errors.New("timed out waiting for the Cryol Agent service")
		}
	}
}

func uninstallService(ctx context.Context) error {
	if !windows.GetCurrentProcessToken().IsElevated() {
		return errors.New("run the uninstall command as an administrator")
	}
	manager, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer manager.Disconnect()
	service, err := manager.OpenService(serviceName)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return nil
	}
	if err != nil {
		return err
	}
	defer service.Close()
	state, err := service.Query()
	if err != nil {
		return err
	}
	if state.State != svc.Stopped {
		if state.State != svc.StopPending {
			if _, err := service.Control(svc.Stop); err != nil && !errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) {
				return err
			}
		}
		if err := waitForService(ctx, service, svc.Stopped); err != nil {
			return err
		}
	}
	if err := service.Delete(); err != nil {
		return err
	}
	fmt.Println("Cryol Agent service removed. Local enrollment files and server history were preserved.")
	if err := eventlog.Remove(serviceName); err != nil && !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return fmt.Errorf("service removed, but its Event Viewer source could not be unregistered: %w", err)
	}
	return nil
}
