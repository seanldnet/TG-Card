package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/seanldnet/TG-Card/internal/auth"
	"github.com/seanldnet/TG-Card/internal/config"
	"github.com/seanldnet/TG-Card/internal/database"
	"golang.org/x/crypto/bcrypt"
)

type application struct {
	db *sql.DB
	driver string
	tokens *auth.Manager
}

type apiEnvelope struct {
	Code int \`json:"code"\`
	Data any \`json:"data"\`
	Error any \`json:"error"\`
	Message string \`json:"message"\`
}

type loginRequest struct {
	Username string \`json:"username"\`
	Password string \`json:"password"\`
}

type userInfo struct {
	ID string \`json:"id"\`
	Username string \`json:"username"\`
	RealName string \`json:"realName"\`
	Roles []string \`json:"roles"\`
	HomePath string \`json:"homePath"\`
}

func main() {
	cfg, err := config.Load()
	if err != nil { log.Fatal(err) }

	ctx := context.Background()
	db, err := database.Open(ctx, cfg)
	if err != nil { log.Fatal(err) }
	defer db.Close()

	if err := database.Migrate(ctx, db, cfg.DBDriver); err != nil { log.Fatal(err) }

	bootstrap, err := database.EnsureAdmin(ctx, db, cfg.DBDriver, cfg.AdminUsername, cfg.AdminPassword)
	if err != nil { log.Fatal(err) }
	if bootstrap.Created {
		if bootstrap.Generated {
			log.Printf("Initial administrator created: username=%s password=%s (shown once; save it securely)", bootstrap.Username, bootstrap.Password)
		} else {
			log.Printf("Initial administrator created from ADMIN_USERNAME/ADMIN_PASSWORD: username=%s", bootstrap.Username)
		}
	}

	tokens, err := auth.NewManager(cfg.JWTSecret, cfg.DataDir)
	if err != nil { log.Fatal(err) }
	app := &application{db: db, driver: cfg.DBDriver, tokens: tokens}

	api := http.NewServeMux()
	api.HandleFunc("GET /api/health", app.health)
	api.HandleFunc("POST /api/auth/login", app.login)
	api.HandleFunc("POST /api/auth/logout", app.logout)
	api.HandleFunc("GET /api/auth/codes", app.accessCodes)
	api.HandleFunc("GET /api/user/info", app.userInfo)
	api.HandleFunc("GET /api/menu/all", app.menus)

	root := http.NewServeMux()
	root.Handle("/api/", api)
	root.Handle("/", staticHandler(cfg.WebDistDir))

	addr := ":" + cfg.Port
	server := &http.Server{
		Addr: addr, Handler: securityHeaders(root),
		ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second,
	}
	log.Printf("TG-Card server listening on %s (database=%s, web_dist=%s)", addr, cfg.DBDriver, cfg.WebDistDir)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) { log.Fatal(err) }
}

func (a *application) health(w http.ResponseWriter, r *http.Request) {
	writeSuccess(w, http.StatusOK, map[string]any{
		"status": "ok", "service": "tg-card", "time": time.Now().UTC().Format(time.RFC3339),
	})
}

func (a *application) login(w http.ResponseWriter, r *http.Request) {
	var input loginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	input.Username = strings.TrimSpace(input.Username)
	if input.Username == "" || input.Password == "" || len(input.Username) > 64 || len(input.Password) > 256 {
		writeError(w, http.StatusBadRequest, "Username and password are required")
		return
	}

	admin, err := database.FindAdminByUsername(r.Context(), a.db, a.driver, input.Username)
	if err != nil || admin.Status != 1 || bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(input.Password)) != nil {
		database.RecordLoginAudit(r.Context(), a.db, a.driver, "", input.Username, clientIP(r), r.UserAgent(), false)
		writeError(w, http.StatusUnauthorized, "Username or password is incorrect")
		return
	}

	token, err := a.tokens.Issue(auth.Claims{
		AdminID: admin.ID, Username: admin.Username, RealName: admin.RealName, Role: admin.Role,
	}, 24*time.Hour)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Unable to create login session")
		return
	}
	if err := database.UpdateLastLogin(r.Context(), a.db, a.driver, admin.ID); err != nil {
		log.Printf("update administrator last login: %v", err)
	}
	database.RecordLoginAudit(r.Context(), a.db, a.driver, admin.ID, admin.Username, clientIP(r), r.UserAgent(), true)
	writeSuccess(w, http.StatusOK, map[string]string{"accessToken": token})
}

func (a *application) logout(w http.ResponseWriter, r *http.Request) {
	writeSuccess(w, http.StatusOK, "")
}

func (a *application) accessCodes(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authenticate(w, r); !ok { return }
	writeSuccess(w, http.StatusOK, []string{})
}

func (a *application) userInfo(w http.ResponseWriter, r *http.Request) {
	claims, ok := a.authenticate(w, r)
	if !ok { return }
	writeSuccess(w, http.StatusOK, userInfo{
		ID: claims.AdminID, Username: claims.Username, RealName: claims.RealName,
		Roles: []string{claims.Role}, HomePath: "/dashboard/analytics",
	})
}

func (a *application) menus(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authenticate(w, r); !ok { return }
	menus := []map[string]any{
		{
			"name": "Dashboard", "path": "/dashboard", "redirect": "/dashboard/analytics",
			"meta": map[string]any{"icon": "lucide:layout-dashboard", "order": -1, "title": "page.dashboard.title"},
			"children": []map[string]any{
				{
					"name": "Analytics", "path": "analytics", "component": "/dashboard/analytics/index",
					"meta": map[string]any{"affixTab": true, "icon": "lucide:area-chart", "title": "page.dashboard.analytics"},
				},
				{
					"name": "Workspace", "path": "workspace", "component": "/dashboard/workspace/index",
					"meta": map[string]any{"icon": "carbon:workspace", "title": "page.dashboard.workspace"},
				},
			},
		},
	}
	writeSuccess(w, http.StatusOK, menus)
}

func (a *application) authenticate(w http.ResponseWriter, r *http.Request) (auth.Claims, bool) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return auth.Claims{}, false
	}
	claims, err := a.tokens.Verify(strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return auth.Claims{}, false
	}
	return claims, true
}

func writeSuccess(w http.ResponseWriter, status int, data any) {
	writeJSON(w, status, apiEnvelope{Code: 0, Data: data, Error: nil, Message: "ok"})
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, apiEnvelope{Code: -1, Data: nil, Error: message, Message: message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func staticHandler(directory string) http.Handler {
	root, err := filepath.Abs(directory)
	if err != nil { return http.NotFoundHandler() }
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		log.Printf("Frontend build directory %q is not present; API remains available until Vben assets are built", root)
		return http.NotFoundHandler()
	}
	fileServer := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath := filepath.Clean("/" + r.URL.Path)
		target := filepath.Join(root, requestPath)
		if info, err := os.Stat(target); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}
		if filepath.Ext(requestPath) != "" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(root, "index.html"))
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil { return host }
	return r.RemoteAddr
}
