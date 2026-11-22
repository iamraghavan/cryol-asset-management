package app

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"

	"github.com/cryol/cryol-asset-management/apps/server/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const sessionCookie = "cryol_session"

type loginBudget struct {
	mu    sync.Mutex
	start time.Time
	count int
	slots chan struct{}
}

func (b *loginBudget) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if time.Since(b.start) >= time.Minute {
		b.start, b.count = time.Now(), 0
	}
	if b.count >= 60 {
		return false
	}
	b.count++
	return true
}

func MountIdentity(r chi.Router, pool *pgxpool.Pool) {
	limit := loginBudget{slots: make(chan struct{}, 4)}
	r.Group(func(r chi.Router) {
		r.Use(http.NewCrossOriginProtection().Handler)
		r.Post("/api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
			var input struct{ Username, Password string }
			if !decode(w, r, &input) {
				return
			}
			select {
			case limit.slots <- struct{}{}:
				defer func() { <-limit.slots }()
			default:
				fail(w, r, 429, "LOGIN_BUSY", "Sign-in is busy. Try again shortly.")
				return
			}
			if !limit.allow() {
				fail(w, r, 429, "LOGIN_BUSY", "Too many sign-in attempts. Try again shortly.")
				return
			}
			token, err := auth.Login(r.Context(), pool, input.Username, input.Password, readToken(r))
			if errors.Is(err, auth.ErrCredentials) {
				fail(w, r, 401, "INVALID_CREDENTIALS", "Username or password is incorrect.")
				return
			}
			if err != nil {
				fail(w, r, 503, "AUTH_UNAVAILABLE", "Sign-in is temporarily unavailable.")
				return
			}
			setCookie(w, r, token, int(auth.SessionLifetime.Seconds()))
			reply(w, 200, map[string]string{"status": "authenticated"})
		})
		r.Post("/api/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
			err := auth.Logout(r.Context(), pool, readToken(r))
			if err != nil && !errors.Is(err, auth.ErrCredentials) {
				fail(w, r, 503, "AUTH_UNAVAILABLE", "Sign-out is temporarily unavailable.")
				return
			}
			setCookie(w, r, "", -1)
			reply(w, 200, map[string]string{"status": "signed-out"})
		})
		r.Get("/api/v1/auth/me", func(w http.ResponseWriter, r *http.Request) {
			if user, ok := authorize(w, r, pool, ""); ok {
				reply(w, 200, user)
			}
		})
		r.Get("/api/v1/organization/profile", func(w http.ResponseWriter, r *http.Request) {
			if _, ok := authorize(w, r, pool, "settings.read"); !ok {
				return
			}
			profile := organization{Timezone: "UTC", Currency: "USD", FinancialYearMonth: 1, AssetPrefix: "CRY"}
			err := pool.QueryRow(r.Context(), `SELECT name,address,contact,timezone,currency,financial_year_month,asset_prefix FROM organization_profile WHERE singleton`).
				Scan(&profile.Name, &profile.Address, &profile.Contact, &profile.Timezone, &profile.Currency, &profile.FinancialYearMonth, &profile.AssetPrefix)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				fail(w, r, 503, "PROFILE_UNAVAILABLE", "Organization profile is temporarily unavailable.")
				return
			}
			reply(w, 200, profile)
		})
		r.Put("/api/v1/organization/profile", func(w http.ResponseWriter, r *http.Request) {
			user, ok := authorize(w, r, pool, "settings.manage")
			if !ok {
				return
			}
			var profile organization
			if !decode(w, r, &profile) {
				return
			}
			profile.Name = strings.TrimSpace(profile.Name)
			if !profile.valid() {
				fail(w, r, 422, "INVALID_PROFILE", "Check organization name, time zone, currency, financial year month and asset prefix.")
				return
			}
			_, err := pool.Exec(r.Context(), `WITH saved AS (
INSERT INTO organization_profile (name,address,contact,timezone,currency,financial_year_month,asset_prefix)
VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (singleton) DO UPDATE SET name=excluded.name,address=excluded.address,contact=excluded.contact,
timezone=excluded.timezone,currency=excluded.currency,financial_year_month=excluded.financial_year_month,
asset_prefix=excluded.asset_prefix,updated_at=now() RETURNING singleton
) INSERT INTO audit_log (actor_id,event) SELECT $8,'organization.profile.updated' FROM saved`,
				profile.Name, profile.Address, profile.Contact, profile.Timezone, profile.Currency, profile.FinancialYearMonth, profile.AssetPrefix, user.ID)
			if err != nil {
				fail(w, r, 503, "PROFILE_UNAVAILABLE", "Organization profile could not be saved.")
				return
			}
			reply(w, 200, profile)
		})
	})
}

type organization struct {
	Name               string `json:"name"`
	Address            string `json:"address"`
	Contact            string `json:"contact"`
	Timezone           string `json:"timezone"`
	Currency           string `json:"currency"`
	FinancialYearMonth int    `json:"financialYearMonth"`
	AssetPrefix        string `json:"assetPrefix"`
}

var currencyFormat = regexp.MustCompile(`^[A-Z]{3}$`)
var prefixFormat = regexp.MustCompile(`^[A-Z0-9-]{1,16}$`)

func (p organization) valid() bool {
	_, tzErr := time.LoadLocation(p.Timezone)
	return len(p.Name) > 0 && len(p.Name) <= 200 && len(p.Address) <= 2000 && len(p.Contact) <= 500 &&
		p.Timezone != "" && p.Timezone != "Local" && tzErr == nil && currencyFormat.MatchString(p.Currency) &&
		p.FinancialYearMonth >= 1 && p.FinancialYearMonth <= 12 && prefixFormat.MatchString(p.AssetPrefix) &&
		!strings.ContainsRune(p.Name+p.Address+p.Contact, 0)
}

func readToken(r *http.Request) string {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func setCookie(w http.ResponseWriter, r *http.Request, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: value, Path: "/", MaxAge: maxAge,
		HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode,
	})
}

func authorize(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool, permission string) (auth.User, bool) {
	user, err := auth.Current(r.Context(), pool, readToken(r))
	switch {
	case errors.Is(err, auth.ErrCredentials):
		fail(w, r, 401, "AUTH_REQUIRED", "Sign in to continue.")
	case err != nil:
		fail(w, r, 503, "AUTH_UNAVAILABLE", "Authentication is temporarily unavailable.")
	case permission != "" && !user.Can(permission):
		fail(w, r, 403, "ACCESS_DENIED", "You do not have permission to perform this operation.")
	default:
		return user, true
	}
	return auth.User{}, false
}

func decode(w http.ResponseWriter, r *http.Request, dest any) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		fail(w, r, 415, "JSON_REQUIRED", "Send an application/json request.")
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768))
	decoder.DisallowUnknownFields()
	if decoder.Decode(dest) != nil || decoder.Decode(new(json.RawMessage)) != io.EOF {
		fail(w, r, 400, "INVALID_REQUEST", "Request contains invalid JSON.")
		return false
	}
	return true
}

func fail(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	reply(w, status, map[string]any{"error": map[string]string{
		"code": code, "message": message, "requestId": middleware.GetReqID(r.Context()),
	}})
}
