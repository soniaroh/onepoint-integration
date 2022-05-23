package main

import (
	"errors"
	"fmt"
	"net/http"

	"opapi"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

type LoginCredentials struct {
	Username     string `json:"username"`
	Password     string `json:"password"`
	CompanyShort string `json:"company_short"`
	IsAdmin      bool   `json:"is_admin"`
}

type LoginResponse struct {
	Token     string `json:"token"`
	UserToken string `json:"user_token"`
}

const adminKey = ""

func registerAuthRoutes(router *gin.Engine) {
	router.POST("/api/login", login)
}

func login(c *gin.Context) {
	input := LoginCredentials{}
	if err := c.ShouldBindWith(&input, binding.JSON); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	thisCompany := &Company{}
	err := dbmap.SelectOne(thisCompany, "SELECT * FROM companies WHERE short_name = ?", input.CompanyShort)
	if err != nil {
		ErrorLog.Println(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "That company is not registered for OnePoint Connect. Please contact OnePoint HCM to authorize your company."})
		return
	}

	inputCreds := opapi.OPCredentials{
		Username:         input.Username,
		Password:         input.Password,
		CompanyShortname: input.CompanyShort,
		APIKey:           thisCompany.APIKey,
	}

	if input.IsAdmin {
		inputCreds.APIKey = passwords.OP_ADMIN_APIKEY
		inputCreds.CompanyShortname = passwords.OP_ADMIN_SHORTNAME
	}

	inputCxn := &opapi.OPConnection{
		Credentials: &inputCreds,
	}

	logPrefix := fmt.Sprintf("LOGIN [username: %s, shortname: %s] -", inputCreds.Username, inputCreds.CompanyShortname)

	err = inputCxn.LoginAndSetToken()
	if err != nil {
		ErrorLog.Println(logPrefix, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := processOPUserLoginV2(input.Username, thisCompany, input.IsAdmin, false)
	if err != nil {
		ErrorLog.Println(logPrefix, "processOPUserLoginV2 err: "+err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"error": "Could not find that user"})
		return
	}

	InfoLog.Println(logPrefix, " SUCCESS")

	resp := LoginResponse{Token: thisCompany.Token, UserToken: user.Token}

	c.JSON(http.StatusOK, resp)
}

func isAdminUser(c *gin.Context) error {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return errors.New("Not authorized")
	}

	user, err := lookupThisUser(c)
	if err != nil {
		return errors.New("Not authorized")
	}

	if !user.IsSystemAdmin {
		return errors.New("Not authorized")
	}

	return nil
}

func lookupCompany(c *gin.Context) (Company, error) {
	authHeader := c.GetHeader("Authorization")
	authHeaderUsed := ""
	if authHeader == "" {
		authADHeader := c.GetHeader("AAD")
		if authADHeader != "" {
			InfoLog.Println("authADHeader:", authADHeader)
			authHeaderUsed = authADHeader
		} else {
			return Company{}, errors.New("No auth header")
		}
	} else {
		authHeaderUsed = authHeader
	}

	thisCompany := Company{}
	err := dbmap.SelectOne(&thisCompany, "SELECT * FROM companies WHERE token=?", authHeaderUsed)
	if err != nil {
		return Company{}, errors.New("Company Not found")
	}

	return thisCompany, nil
}

func lookupThisUser(c *gin.Context) (*User, error) {
	authHeader := c.GetHeader("OPUserToken")
	if authHeader == "" {
		return nil, errors.New("Not authorized")
	}

	thisUser := User{}
	err := dbmap.SelectOne(&thisUser, "SELECT * FROM users WHERE token=?", authHeader)
	if err != nil {
		return nil, err
	}

	return &thisUser, nil
}

func lookupUserAndCompany(c *gin.Context) (*User, *Company, error) {
	co, err := lookupCompany(c)
	if err != nil {
		return nil, nil, err
	}
	user, err := lookupThisUser(c)
	if err != nil {
		return nil, nil, err
	}
	return user, &co, nil
}
