package auth

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/hveda/setagaya/setagaya/config"
	ldap "gopkg.in/ldap.v2"
)

var (
	CNPattern  = regexp.MustCompile(`CN=([^,]+)\,OU=DLM\sDistribution\sGroups`)
	AccountKey = "account"
	MLKey      = "ml"
)

type AuthResult struct {
	ML []string
}

func Auth(username, password string) (*AuthResult, error) {
	r := new(AuthResult)
	ac := config.SC.AuthConfig
	
	// Simple local authentication when LDAP is not configured
	if ac.LdapServer == "" {
		// Specific user credentials for local testing (as documented in README)
		userCredentials := map[string]string{
			"admin":   "admin123",
			"manager": "manager123", 
			"tester":  "tester123",
			"monitor": "monitor123",
			"setagaya": "admin", // Keep original for backward compatibility
		}
		
		if expectedPassword, exists := userCredentials[username]; exists && password == expectedPassword {
			r.ML = []string{username}
			return r, nil
		}
		
		// Legacy support: Allow admin users from config with "admin" password
		for _, adminUser := range ac.AdminUsers {
			if username == adminUser && password == "admin" {
				r.ML = []string{username}
				return r, nil
			}
		}
		
		// Legacy support: Allow any user with password "password" for development
		if password == "password" {
			r.ML = []string{username}
			return r, nil
		}
		
		return nil, errors.New("Invalid username or password")
	}
	
	// Original LDAP authentication
	ldapServer := ac.LdapServer
	ldapPort := ac.LdapPort
	baseDN := ac.BaseDN
	r.ML = []string{}

	filter := "(&(objectClass=user)(sAMAccountName=%s))"
	l, err := ldap.Dial("tcp", fmt.Sprintf("%s:%s", ldapServer, ldapPort))
	if err != nil {
		return r, err
	}
	defer l.Close()
	systemUser := ac.SystemUser
	systemPassword := ac.SystemPassword
	err = l.Bind(systemUser, systemPassword)
	if err != nil {
		return r, err
	}
	searchRequest := ldap.NewSearchRequest(
		baseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf(filter, username),
		[]string{"userprincipalname"},
		nil,
	)
	sr, err := l.Search(searchRequest)
	if err != nil {
		return r, err
	}

	entries := sr.Entries
	if len(entries) != 1 {
		return r, errors.New("Users does not exist")
	}

	attributes := entries[0].Attributes
	if len(attributes) == 0 {
		return r, errors.New("Cannot find the user")
	}
	values := attributes[0].Values
	if len(values) == 0 {
		return r, errors.New("Cannot find the principle name")
	}
	UserPrincipalName := values[0]
	err = l.Bind(UserPrincipalName, password)
	if err != nil {
		return r, errors.New("Incorrect password")
	}
	searchRequest = ldap.NewSearchRequest(
		baseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf(filter, username),
		[]string{"memberOf"},
		nil,
	)
	sr, err = l.Search(searchRequest)
	if err != nil {
		return r, errors.New("Error in contacting LDAP server")
	}
	entries = sr.Entries
	if len(entries) == 0 {
		return r, errors.New("Cannot find user ml/group information")
	}
	attributes = entries[0].Attributes
	if len(attributes) == 0 {
		return r, errors.New("Cannot find user group/ml information")
	}
	values = attributes[0].Values
	for _, m := range values {
		match := CNPattern.FindStringSubmatch(m)
		if match == nil {
			continue
		}
		r.ML = append(r.ML, match[1])
	}
	return r, nil
}
