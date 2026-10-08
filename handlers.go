package main

import (
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

// App wires the catalogue, dossier store and renderer to HTTP.
type App struct {
	catalog  *Catalog
	dossiers *DossierStore
	render   *Renderer
	assets   *AssetStore
	log      *slog.Logger
	now      func() time.Time
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", a.vault)
	mux.HandleFunc("GET /ledger", a.ledger)
	mux.HandleFunc("GET /ledger/{slug}", a.ledgerEntry)
	mux.HandleFunc("GET /commission", a.commissionForm)
	mux.HandleFunc("POST /commission", a.commissionSubmit)
	mux.HandleFunc("GET /commission/received/{ref}", a.commissionReceived)
	mux.Handle("GET /static/{file...}", a.assets)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/", a.notFound)
	return a.recoverPanics(a.logRequests(securityHeaders(mux)))
}

// Views

type VaultView struct {
	Page
	Hero       template.HTML
	Flagship   Edition
	Editions   []Edition
	Recent     []LedgerEntry
	LunarDrift string
}

type LedgerView struct {
	Page
	Entries   []LedgerEntry
	Total     int
	Query     LedgerQuery
	Columns   []SortColumn
	SortLabel string
	Tiers     []Tier
}

type EntryView struct {
	Page
	Entry LedgerEntry
	Prev  *LedgerEntry
	Next  *LedgerEntry
}

type StageTab struct {
	N        int
	Name     string
	HasError bool
}

type CommissionView struct {
	Page
	Input        CommissionInput
	Errors       FieldErrors
	ErrorList    []FieldError
	Stage        int
	Stages       []StageTab
	CSRF         string
	Tiers        []Tier
	Alloys       []Alloy
	Scripts      []Script
	Contacts     []ContactMethod
	TympanCHF    int
	MaxTympans   int
	PreviewNorth template.HTML
	PreviewSouth template.HTML
}

type ReviewView struct {
	Page
	C     Commission
	Input CommissionInput
	CSRF  string
	Plate template.HTML
}

type ReceivedView struct {
	Page
	C     Commission
	Plate template.HTML
}

type ErrorView struct {
	Page
	Status  int
	Message string
}

// Handlers

func (a *App) vault(w http.ResponseWriter, r *http.Request) {
	eds := a.catalog.Editions
	flagship := eds[0]
	view := VaultView{
		Page: Page{
			Title:       "The Vault",
			Description: "Planispheric astrolabes and celestial chronometers, drawn for one latitude and made by hand in Geneva.",
			Section:     "vault",
		},
		Hero: RenderPlate(PlateOptions{
			ID: "plate-hero", Title: fmt.Sprintf("Tympan and rete of %s, drawn for %s", flagship.Name, flagship.LatitudeLabel()),
			Latitude: flagship.Latitude, Southern: flagship.Hemisphere == Southern,
			AlmucantarStep: flagship.AlmucantarStep, AzimuthStep: flagship.AzimuthStep,
		}),
		Flagship: flagship,
		Editions: eds,
		Recent:   a.catalog.Recent(4),
	}
	for _, e := range eds {
		if t := e.Synodic(); t != nil {
			view.LunarDrift = formatYears(t.YearsPerDayOfError())
			break
		}
	}
	a.render.Render(w, r, http.StatusOK, "vault", view)
}

func (a *App) ledger(w http.ResponseWriter, r *http.Request) {
	q := ParseLedgerQuery(r.URL.Query())
	a.render.Render(w, r, http.StatusOK, "ledger", LedgerView{
		Page: Page{
			Title:       "The Ephemeris Ledger",
			Description: "Every instrument delivered by Atelier Copernic since 1987, with caliber, latitude and certificate of provenance.",
			Section:     "ledger",
		},
		Entries:   a.catalog.SearchLedger(q),
		Total:     len(a.catalog.Ledger),
		Query:     q,
		Columns:   ledgerColumns(q),
		SortLabel: sortLabel(q.Sort),
		Tiers:     tiers,
	})
}

func (a *App) ledgerEntry(w http.ResponseWriter, r *http.Request) {
	entry, prev, next, ok := a.catalog.Entry(r.PathValue("slug"))
	if !ok {
		a.notFound(w, r)
		return
	}
	a.render.Render(w, r, http.StatusOK, "entry", EntryView{
		Page: Page{
			Title:       fmt.Sprintf("%s, %s", entry.Name, entry.Ref),
			Description: entry.Summary,
			Section:     "ledger",
		},
		Entry: entry, Prev: prev, Next: next,
	})
}

func (a *App) commissionForm(w http.ResponseWriter, r *http.Request) {
	a.renderCommission(w, r, http.StatusOK, CommissionInput{}, nil, 1)
}

func (a *App) renderCommission(w http.ResponseWriter, r *http.Request, status int, in CommissionInput, errs FieldErrors, stage int) {
	token, err := csrfToken(w, r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	stages := make([]StageTab, stageCount)
	for i := range stages {
		stages[i] = StageTab{N: i + 1, Name: stageNames[i+1], HasError: errs.StageHasError(i + 1)}
	}
	north, south := a.previewPlates()
	w.Header().Set("Cache-Control", "no-store")
	a.render.Render(w, r, status, "commission", CommissionView{
		Page: Page{
			Title:       "Commission Dossier",
			Description: "Open a private commission: choose the sky, the complication and the engraving for an instrument made for you.",
			Section:     "commission",
		},
		Input: in, Errors: errs, ErrorList: errs.List(),
		Stage: stage, Stages: stages, CSRF: token,
		Tiers: tiers, Alloys: alloys, Scripts: scripts, Contacts: contactMethods,
		TympanCHF: tympanCHF, MaxTympans: maxTympans,
		PreviewNorth: north, PreviewSouth: south,
	})
}

// previewPlates reuses the first northern and southern edition drawings.
func (a *App) previewPlates() (north, south template.HTML) {
	for _, e := range a.catalog.Editions {
		if e.Hemisphere == Southern && south == "" {
			south = e.Plate
		}
		if e.Hemisphere == Northern && north == "" {
			north = e.Plate
		}
	}
	return north, south
}

func (a *App) commissionSubmit(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCommissionBody)
	if err := r.ParseForm(); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			a.renderError(w, r, http.StatusRequestEntityTooLarge, "The dossier was larger than the atelier accepts. Please shorten your notes.")
			return
		}
		a.renderError(w, r, http.StatusBadRequest, "The dossier could not be read. Please try again.")
		return
	}
	if !validCSRF(r) {
		a.renderError(w, r, http.StatusForbidden, "This dossier form has expired. Reload the commission page and submit it again.")
		return
	}

	in := readCommissionInput(r)
	action := r.PostFormValue("action")
	switch action {
	case "amend":
		a.renderCommission(w, r, http.StatusOK, in, nil, stageCount)
		return
	case "review", "confirm":
	default:
		a.renderError(w, r, http.StatusBadRequest, "Unknown dossier action.")
		return
	}

	c, errs := in.Validate()
	if len(errs) > 0 {
		a.renderCommission(w, r, http.StatusUnprocessableEntity, in, errs, errs.FirstStage())
		return
	}

	if action == "review" {
		w.Header().Set("Cache-Control", "no-store")
		a.render.Render(w, r, http.StatusOK, "review", ReviewView{
			Page:  Page{Title: "Review Dossier", Description: "Review your commission before lodging it.", Section: "commission"},
			C:     c,
			Input: in,
			CSRF:  r.PostFormValue("csrf"),
			Plate: commissionPlate("plate-review", c),
		})
		return
	}

	ref, err := a.dossiers.Save(c, a.now())
	if errors.Is(err, errStoreFull) {
		a.renderError(w, r, http.StatusServiceUnavailable, "The atelier is not accepting new dossiers online at the moment. Please write to the atelier directly.")
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.log.Info("dossier lodged", "ref", ref, "tier", c.Tier.ID)
	http.Redirect(w, r, "/commission/received/"+ref, http.StatusSeeOther)
}

func commissionPlate(id string, c Commission) template.HTML {
	return RenderPlate(PlateOptions{
		ID:       id,
		Title:    "Your tympan, drawn for " + c.LatitudeLabel(),
		Latitude: c.Latitude(),
		Southern: c.Hemisphere == Southern,
	})
}

// commissionReceived shows only non-personal details: the reference in the
// URL is unguessable but may be shared, so names and addresses stay out.
func (a *App) commissionReceived(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("ref")
	if !dossierRefPattern.MatchString(ref) {
		a.notFound(w, r)
		return
	}
	c, ok := a.dossiers.Get(ref)
	if !ok {
		a.notFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	a.render.Render(w, r, http.StatusOK, "received", ReceivedView{
		Page:  Page{Title: "Dossier Received", Description: "Your commission dossier has been lodged with the atelier.", Section: "commission"},
		C:     c,
		Plate: commissionPlate("plate-received", c),
	})
}

func (a *App) notFound(w http.ResponseWriter, r *http.Request) {
	a.renderError(w, r, http.StatusNotFound, "There is no page at this address. The ledger may list what you were looking for.")
}

func (a *App) serverError(w http.ResponseWriter, r *http.Request, err error) {
	a.log.Error("server error", "path", r.URL.Path, "err", err)
	a.renderError(w, r, http.StatusInternalServerError, "Something went wrong on our side. Please try again in a moment.")
}

func (a *App) renderError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	w.Header().Set("Cache-Control", "no-store")
	a.render.Render(w, r, status, "error", ErrorView{
		Page:    Page{Title: http.StatusText(status), Description: msg},
		Status:  status,
		Message: msg,
	})
}

// Middleware

// securityHeaders sets a policy that forbids every script source, so the
// zero-JavaScript constraint is enforced by the browser, not just by habit.
func securityHeaders(next http.Handler) http.Handler {
	csp := strings.Join([]string{
		"default-src 'none'",
		"script-src 'none'",
		"style-src 'self' https://fonts.googleapis.com",
		"font-src https://fonts.gstatic.com",
		"img-src 'self' data:",
		"form-action 'self'",
		"base-uri 'none'",
		"frame-ancestors 'none'",
	}, "; ")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), interest-cohort=()")
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

func (a *App) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		a.log.Info("request", "method", r.Method, "path", r.URL.Path, "status", rec.status, "dur", time.Since(start).Round(time.Microsecond))
	})
}

func (a *App) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				a.log.Error("panic", "path", r.URL.Path, "value", v, "stack", string(debug.Stack()))
				w.Header().Set("Connection", "close")
				a.renderError(w, r, http.StatusInternalServerError, "Something went wrong on our side. Please try again in a moment.")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
