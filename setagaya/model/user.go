package model

import (
	"net/http"

	"github.com/hveda/Setagaya/setagaya/auth"
	"github.com/hveda/Setagaya/setagaya/config"
	log "github.com/sirupsen/logrus"
)

type Account struct {
	ML    []string
	MLMap map[string]interface{}
	Name  string
}

var es interface{}

func GetAccountBySession(r *http.Request) *Account {
	a := new(Account)
	a.MLMap = make(map[string]interface{})
	if config.SC.AuthConfig.NoAuth {
		a.Name = "setagaya"
		a.ML = []string{a.Name}
		a.MLMap[a.Name] = es
		return a
	}
	session, err := auth.SessionStore.Get(r, config.SC.AuthConfig.SessionKey)
	if err != nil {
		return nil
	}
	accountName := session.Values[auth.AccountKey]
	if accountName == nil {
		return nil
	}
	if name, ok := accountName.(string); ok {
		a.Name = name
	} else {
		log.Printf("Error: accountName is not string: %v", accountName)
		return nil
	}
	if ml, ok := session.Values[auth.MLKey].([]string); ok {
		a.ML = ml
	} else {
		log.Printf("Error: ML value is not []string: %v", session.Values[auth.MLKey])
		a.ML = []string{} // default to empty slice
	}
	for _, m := range a.ML {
		a.MLMap[m] = es
	}
	return a
}

func (a *Account) IsAdmin() bool {
	for _, ml := range a.ML {
		for _, admin := range config.SC.AuthConfig.AdminUsers {
			if ml == admin {
				return true
			}
		}
	}
	// systemuser is the user used for LDAP auth. If a user login with that account
	// we can also treat it as a admin
	if a.Name == config.SC.AuthConfig.SystemUser {
		return true
	}
	return false
}
