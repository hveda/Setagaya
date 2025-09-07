package ui

import (
	"fmt"
	"html/template"
	"net/http"

	"github.com/julienschmidt/httprouter"
	log "github.com/sirupsen/logrus"

	"github.com/hveda/Setagaya/setagaya/api"
	"github.com/hveda/Setagaya/setagaya/auth"
	"github.com/hveda/Setagaya/setagaya/config"
	"github.com/hveda/Setagaya/setagaya/model"
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
	if err := template.Execute(w, &HomeResp{account.Name, sc.BackgroundColour, sc.Context,
		IsAdmin, resultDashboardURL, enableSid,
		engineHealthDashboardURL, sc.ProjectHome, sc.UploadFileHelp, gcDuration}); err != nil {
		log.Printf("Error executing home template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

func (u *UI) loginHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	if err := r.ParseForm(); err != nil {
		log.Printf("Error parsing form: %v", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	ss := auth.SessionStore
	session, err := ss.Get(r, config.SC.AuthConfig.SessionKey)
	if err != nil {
		log.Print(err)
	}
	username := r.Form.Get("username")
	password := r.Form.Get("password")
	authResult, err := auth.Auth(username, password)
	if err != nil {
		loginUrl := fmt.Sprintf("/login?error_msg=%v", err)
		http.Redirect(w, r, loginUrl, http.StatusSeeOther)
	}
	session.Values[auth.MLKey] = authResult.ML
	session.Values[auth.AccountKey] = username
	err = ss.Save(r, w, session)
	if err != nil {
		log.Panic(err)
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (u *UI) logoutHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	session, err := auth.SessionStore.Get(r, config.SC.AuthConfig.SessionKey)
	if err != nil {
		log.Print(err)
		return
	}
	delete(session.Values, auth.MLKey)
	delete(session.Values, auth.AccountKey)
	if err := session.Save(r, w); err != nil {
		log.Printf("Error saving session: %v", err)
	}
}

type LoginResp struct {
	ErrorMsg string
}

func (u *UI) loginPageHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	template := u.tmpl.Lookup("login.html")
	qs := r.URL.Query()
	errMsgs := qs["error_msg"]
	e := new(LoginResp)
	e.ErrorMsg = ""
	if len(errMsgs) > 0 {
		e.ErrorMsg = errMsgs[0]
	}
	if err := template.Execute(w, e); err != nil {
		log.Printf("Error executing login template: %v", err)
	}
}

func (u *UI) InitRoutes() api.Routes {
	return api.Routes{
		&api.Route{Name: "home", Method: "GET", Path: "/", HandlerFunc: u.homeHandler},
		&api.Route{Name: "login", Method: "POST", Path: "/login", HandlerFunc: u.loginHandler},
		&api.Route{Name: "login", Method: "GET", Path: "/login", HandlerFunc: u.loginPageHandler},
		&api.Route{Name: "logout", Method: "POST", Path: "/logout", HandlerFunc: u.logoutHandler},
	}
}
