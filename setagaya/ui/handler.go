package ui

import (
	"fmt"
	"html/template"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/hveda/setagaya/setagaya/api"
	"github.com/hveda/setagaya/setagaya/auth"
	"github.com/hveda/setagaya/setagaya/config"
	"github.com/hveda/setagaya/setagaya/model"
	log "github.com/sirupsen/logrus"
)

type UI struct {
	tmpl   *template.Template
	Routes []*api.Route
}

func NewUI() *UI {
	u := &UI{
		tmpl: template.Must(template.ParseGlob("/templates/*.html")),
	}
	return u
}

type HomeResp struct {
	Account               string
	BackgroundColour      string
	Context               string
	IsAdmin               bool
	ResultDashboard       string
	EnableSid             bool
	EngineHealthDashboard string
	ProjectHome           string
	UploadFileHelp        string
	GCDuration            float64
}

func (u *UI) homeHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	account := model.GetAccountBySession(r)
	if account == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	IsAdmin := account.IsAdmin()
	enableSid := config.SC.EnableSid
	resultDashboardURL := config.SC.DashboardConfig.Url + config.SC.DashboardConfig.RunDashboard
	engineHealthDashboardURL := config.SC.DashboardConfig.Url + config.SC.DashboardConfig.EnginesDashboard
	if config.SC.DashboardConfig.EnginesDashboard == "" {
		engineHealthDashboardURL = ""
	}
	template := u.tmpl.Lookup("app.html")
	sc := config.SC
	gcDuration := config.SC.ExecutorConfig.Cluster.GCDuration
	template.Execute(w, &HomeResp{account.Name, sc.BackgroundColour, sc.Context,
		IsAdmin, resultDashboardURL, enableSid,
		engineHealthDashboardURL, sc.ProjectHome, sc.UploadFileHelp, gcDuration})
}

func (u *UI) loginHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	r.ParseForm()
	
	// In no-auth mode, skip authentication and redirect to home
	if config.SC.AuthConfig.NoAuth {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	
	ss := auth.SessionStore
	if ss == nil {
		http.Error(w, "Session store not initialized", http.StatusInternalServerError)
		return
	}
	
	session, err := ss.Get(r, config.SC.AuthConfig.SessionKey)
	if err != nil {
		log.Print(err)
		http.Error(w, "Session error", http.StatusInternalServerError)
		return
	}
	username := r.Form.Get("username")
	password := r.Form.Get("password")
	authResult, err := auth.Auth(username, password)
	if err != nil {
		loginUrl := fmt.Sprintf("/login?error_msg=%v", err)
		http.Redirect(w, r, loginUrl, http.StatusSeeOther)
		return
	}
	if authResult == nil {
		loginUrl := "/login?error_msg=Authentication failed"
		http.Redirect(w, r, loginUrl, http.StatusSeeOther)
		return
	}
	session.Values[auth.MLKey] = authResult.ML
	session.Values[auth.AccountKey] = username
	err = ss.Save(r, w, session)
	if err != nil {
		log.Print(err)
		http.Error(w, "Failed to save session", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (u *UI) logoutHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	// In no-auth mode, just redirect to login
	if config.SC.AuthConfig.NoAuth {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	
	if auth.SessionStore == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	
	session, err := auth.SessionStore.Get(r, config.SC.AuthConfig.SessionKey)
	if err != nil {
		log.Print(err)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	delete(session.Values, auth.MLKey)
	delete(session.Values, auth.AccountKey)
	session.Save(r, w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

type LoginResp struct {
	ErrorMsg string
}

func (u *UI) loginPageHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	// In no-auth mode, redirect to home instead of showing login form
	if config.SC.AuthConfig.NoAuth {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	
	template := u.tmpl.Lookup("login.html")
	qs := r.URL.Query()
	errMsgs := qs["error_msg"]
	e := new(LoginResp)
	e.ErrorMsg = ""
	if len(errMsgs) > 0 {
		e.ErrorMsg = errMsgs[0]
	}
	template.Execute(w, e)
}

func (u *UI) rbacTestHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	template := u.tmpl.Lookup("app-rbac-test.html")
	if template == nil {
		http.Error(w, "Template not found", http.StatusNotFound)
		return
	}
	template.Execute(w, nil)
}

func (u *UI) adminHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	account := model.GetAccountBySession(r)
	if account == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	
	// Check if user has admin permissions
	if !account.IsAdmin() {
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}
	
	// Use the admin interface template
	template := u.tmpl.Lookup("admin-interface.html")
	if template == nil {
		http.Error(w, "Admin template not found", http.StatusNotFound)
		return
	}
	template.Execute(w, nil)
}

func (u *UI) projectsHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	account := model.GetAccountBySession(r)
	if account == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	
	// Use the same template data as home page for now
	IsAdmin := account.IsAdmin()
	enableSid := config.SC.EnableSid
	resultDashboardURL := config.SC.DashboardConfig.Url + config.SC.DashboardConfig.RunDashboard
	engineHealthDashboardURL := config.SC.DashboardConfig.Url + config.SC.DashboardConfig.EnginesDashboard
	if config.SC.DashboardConfig.EnginesDashboard == "" {
		engineHealthDashboardURL = ""
	}
	template := u.tmpl.Lookup("app.html")
	sc := config.SC
	gcDuration := config.SC.ExecutorConfig.Cluster.GCDuration
	template.Execute(w, &HomeResp{account.Name, sc.BackgroundColour, sc.Context,
		IsAdmin, resultDashboardURL, enableSid,
		engineHealthDashboardURL, sc.ProjectHome, sc.UploadFileHelp, gcDuration})
}

func (u *UI) projectDetailHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	account := model.GetAccountBySession(r)
	if account == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	
	// For now, use the same template as home page but we could create a project-specific template
	IsAdmin := account.IsAdmin()
	enableSid := config.SC.EnableSid
	resultDashboardURL := config.SC.DashboardConfig.Url + config.SC.DashboardConfig.RunDashboard
	engineHealthDashboardURL := config.SC.DashboardConfig.Url + config.SC.DashboardConfig.EnginesDashboard
	if config.SC.DashboardConfig.EnginesDashboard == "" {
		engineHealthDashboardURL = ""
	}
	template := u.tmpl.Lookup("app.html")
	sc := config.SC
	gcDuration := config.SC.ExecutorConfig.Cluster.GCDuration
	template.Execute(w, &HomeResp{account.Name, sc.BackgroundColour, sc.Context,
		IsAdmin, resultDashboardURL, enableSid,
		engineHealthDashboardURL, sc.ProjectHome, sc.UploadFileHelp, gcDuration})
}

func (u *UI) collectionsHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	account := model.GetAccountBySession(r)
	if account == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	
	// Use the same template data as home page for now
	IsAdmin := account.IsAdmin()
	enableSid := config.SC.EnableSid
	resultDashboardURL := config.SC.DashboardConfig.Url + config.SC.DashboardConfig.RunDashboard
	engineHealthDashboardURL := config.SC.DashboardConfig.Url + config.SC.DashboardConfig.EnginesDashboard
	if config.SC.DashboardConfig.EnginesDashboard == "" {
		engineHealthDashboardURL = ""
	}
	template := u.tmpl.Lookup("app.html")
	sc := config.SC
	gcDuration := config.SC.ExecutorConfig.Cluster.GCDuration
	template.Execute(w, &HomeResp{account.Name, sc.BackgroundColour, sc.Context,
		IsAdmin, resultDashboardURL, enableSid,
		engineHealthDashboardURL, sc.ProjectHome, sc.UploadFileHelp, gcDuration})
}

func (u *UI) adminInterfaceHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	template := u.tmpl.Lookup("admin-interface.html")
	if template == nil {
		http.Error(w, "Template not found", http.StatusNotFound)
		return
	}
	template.Execute(w, nil)
}

func (u *UI) catchAllHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	account := model.GetAccountBySession(r)
	if account == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	
	// Serve the same app.html for all SPA routes
	IsAdmin := account.IsAdmin()
	enableSid := config.SC.EnableSid
	resultDashboardURL := config.SC.DashboardConfig.Url + config.SC.DashboardConfig.RunDashboard
	engineHealthDashboardURL := config.SC.DashboardConfig.Url + config.SC.DashboardConfig.EnginesDashboard
	if config.SC.DashboardConfig.EnginesDashboard == "" {
		engineHealthDashboardURL = ""
	}
	template := u.tmpl.Lookup("app.html")
	sc := config.SC
	gcDuration := config.SC.ExecutorConfig.Cluster.GCDuration
	template.Execute(w, &HomeResp{account.Name, sc.BackgroundColour, sc.Context,
		IsAdmin, resultDashboardURL, enableSid,
		engineHealthDashboardURL, sc.ProjectHome, sc.UploadFileHelp, gcDuration})
}

func (u *UI) InitRoutes() api.Routes {
	return api.Routes{
		&api.Route{Name: "home", Method: "GET", Path: "/", HandlerFunc: u.homeHandler},
		&api.Route{Name: "login", Method: "POST", Path: "/login", HandlerFunc: u.loginHandler},
		&api.Route{Name: "login", Method: "GET", Path: "/login", HandlerFunc: u.loginPageHandler},
		&api.Route{Name: "logout", Method: "POST", Path: "/logout", HandlerFunc: u.logoutHandler},
		
		// Production routes
		&api.Route{Name: "admin", Method: "GET", Path: "/admin", HandlerFunc: u.adminHandler},
		&api.Route{Name: "projects", Method: "GET", Path: "/projects", HandlerFunc: u.projectsHandler},
		&api.Route{Name: "project_detail", Method: "GET", Path: "/projects/:project_id", HandlerFunc: u.projectDetailHandler},
		&api.Route{Name: "collections", Method: "GET", Path: "/collections", HandlerFunc: u.collectionsHandler},
		
		// SPA catch-all routes for client-side routing
		&api.Route{Name: "plans_list", Method: "GET", Path: "/plans", HandlerFunc: u.catchAllHandler},
		&api.Route{Name: "plan_detail", Method: "GET", Path: "/plans/:plan_id", HandlerFunc: u.catchAllHandler},
		&api.Route{Name: "collection_detail", Method: "GET", Path: "/collections/:collection_id", HandlerFunc: u.catchAllHandler},
		&api.Route{Name: "monitoring", Method: "GET", Path: "/monitoring", HandlerFunc: u.catchAllHandler},
		
		// RBAC and Admin routes
		&api.Route{Name: "rbac_test", Method: "GET", Path: "/app-rbac-test", HandlerFunc: u.rbacTestHandler},
		&api.Route{Name: "admin_interface", Method: "GET", Path: "/admin-interface", HandlerFunc: u.adminInterfaceHandler},
	}
}
