using System.Diagnostics;
using System.Reflection;

if (args.Contains("--help"))
{
    Console.WriteLine("CryolSetup.exe [-ProjectPath PATH] [-DatabasePort 5432] [-AppPort 8080] [-ResetPostgresPassword]");
    Console.WriteLine("Keep the entire extracted download together. Passwords are requested by PowerShell.");
    return 0;
}
string directory = Path.Combine(Path.GetTempPath(), "CryolSetup-" + Guid.NewGuid().ToString("N"));
Directory.CreateDirectory(directory);
try
{
    string script = Path.Combine(directory, "Setup-Cryol.ps1");
    using (Stream resource = Assembly.GetExecutingAssembly()
        .GetManifestResourceStream("CryolSetup.Setup-Cryol.ps1")
        ?? throw new InvalidOperationException("Embedded setup script is missing."))
    using (FileStream output = File.Create(script))
        resource.CopyTo(output);
    var start = new ProcessStartInfo
    {
        FileName = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.System),
            @"WindowsPowerShell\v1.0\powershell.exe"),
        UseShellExecute = false
    };
    foreach (string arg in new[] { "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script })
        start.ArgumentList.Add(arg);
    foreach (string arg in args)
        start.ArgumentList.Add(arg);
    using Process child = Process.Start(start)
        ?? throw new InvalidOperationException("Could not start Windows PowerShell.");
    child.WaitForExit();
    if (child.ExitCode != 0)
    {
        Console.Error.WriteLine("Setup did not finish. Review the error above.");
        Console.WriteLine("Press Enter to close.");
        Console.ReadLine();
    }
    return child.ExitCode;
}
catch (Exception error)
{
    Console.Error.WriteLine(error.Message);
    Console.WriteLine("Press Enter to close.");
    Console.ReadLine();
    return 1;
}
finally
{
    // This invocation owns this newly created temporary directory.
    Directory.Delete(directory, recursive: true);
}
