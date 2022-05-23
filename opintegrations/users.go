package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"opapi"
)

type User struct {
	ID             int64  `db:"id, primarykey, autoincrement"`
	OPID           int64  `db:"op_id" json:"op_id"`
	Username       string `db:"username" json:"username"`
	FirstName      string `db:"first_name" json:"first_name"`
	LastName       string `db:"last_name" json:"last_name"`
	Email          string `db:"email" json:"email"`
	CompanyID      int64  `db:"company_id" json:"company_id"`
	Token          string `db:"token" json:"-"`
	IsSystemAdmin  bool   `db:"is_system_admin" json:"is_system_admin"`
	IsCompanyAdmin bool   `db:"is_company_admin" json:"is_company_admin"`
}

type OPMeResponse struct {
	ID         int64  `json:"id"`
	EmployeeID string `json:"employee_id"`
	Username   string `json:"username"`
	FirstName  string `json:"first_name"`
	LastName   string `json:"last_name"`
	Links      struct {
		Self         string `json:"self"`
		Demographics string `json:"demographics"`
		PayInfo      string `json:"pay-info"`
		Badges       string `json:"badges"`
		Profiles     string `json:"profiles"`
	} `json:"_links"`
}

type CreateUserRequest struct {
	Username       string `json:"username"`
	CompanyID      int64  `json:"company_id"`
	IsCompanyAdmin bool   `json:"is_company_admin"`
	IsSystemAdmin  bool   `json:"is_system_admin"`
}

// Used to generate a User, if isAdminUser = true then company does not matter and can be nil
func processOPUserLoginV2(inputUsername string, company *Company, isAdminUser bool, isCompanyAdmin bool) (*User, error) {
	companyIDtoUse := 0
	if !isAdminUser {
		companyIDtoUse = int(company.ID)
	}

	thisUser := User{}
	err := dbmap.SelectOne(&thisUser, "SELECT * FROM users WHERE username = ? AND company_id = ?", inputUsername, companyIDtoUse)
	if err == nil {
		return &thisUser, nil
	}

	accountId := 0
	companyId := 0
	firstName := ""
	lastName := ""
	email := ""
	isSystemAdmin := false
	if !isAdminUser {
		cxn := chooseOPAPICxn(company.OPID)
		allEmployees, err := cxn.GetAllCompanyEmployees(company.OPID)
		if err != nil {
			return nil, err
		}

		var foundEmployeeShortened *opapi.OPFullEmployeeRecord
		for _, emp := range allEmployees {
			if strings.ToLower(emp.Username) == strings.ToLower(inputUsername) {
				foundEmployeeShortened = &emp
				break
			}
		}

		if foundEmployeeShortened == nil {
			return nil, errors.New("could not find employee by username")
		}

		fullEmployeeRecord, err := cxn.GetFullEmployeeInfo(int64(foundEmployeeShortened.ID), company.OPID)
		if err != nil {
			return nil, err
		}

		companyId = int(company.ID)
		accountId = fullEmployeeRecord.ID
		firstName = fullEmployeeRecord.FirstName
		lastName = fullEmployeeRecord.LastName
		email = fullEmployeeRecord.PrimaryEmail
	} else {
		isSystemAdmin = true
	}

	newToken, err := generateNewToken()
	if err != nil {
		return nil, err
	}

	token := fmt.Sprintf("USERKEY%s%d", newToken, time.Now().Unix())

	newUser := User{
		OPID:           int64(accountId),
		Username:       inputUsername,
		FirstName:      firstName,
		LastName:       lastName,
		Email:          email,
		CompanyID:      int64(companyId),
		Token:          token,
		IsCompanyAdmin: isCompanyAdmin,
		IsSystemAdmin:  isSystemAdmin,
	}

	err = dbmap.Insert(&newUser)
	if err != nil {
		return nil, err
	}

	return &newUser, nil
}

func processOPUserLogin(userCXN *opapi.OPConnection, authToken string) {
	company, err := lookupCompanyByShortname(userCXN.Credentials.CompanyShortname)
	if err != nil {
		ErrorLog.Println("err looking up company by shortname: ", err)
		ErrorLog.Println("username: ", userCXN.Credentials.Username, " coShortName: ", userCXN.Credentials.CompanyShortname)
		return
	}

	existingUser := User{}
	err = dbmap.SelectOne(&existingUser, "SELECT * FROM users WHERE company_id = ? AND username = ?", company.ID, userCXN.Credentials.Username)
	if err == nil {
		return
	}

	opResp := &OPMeResponse{}
	url := fmt.Sprintf("https://secure.onehcm.com/ta/rest/v2/companies/|%s/employees/me", userCXN.Credentials.CompanyShortname)
	err = userCXN.BasicQuery(url, opResp, false)
	if err != nil {
		ErrorLog.Printf("err getting ME info: %v", err)
		return
	}

	newUserFull := &opapi.OPFullEmployeeRecord{}
	thisCo, _ := lookupCompanyByShortname(userCXN.Credentials.CompanyShortname)
	cxn := chooseOPAPICxn(thisCo.OPID)
	err = cxn.BasicQuery(opResp.Links.Self, newUserFull, false)
	if err != nil {
		ErrorLog.Println("op could not access logged in user's EMAIL:", userCXN.Credentials.Username, " csn: ", userCXN.Credentials.CompanyShortname, " err: ", err)
		return
	}

	newUser := User{
		OPID:      opResp.ID,
		Username:  opResp.Username,
		FirstName: opResp.FirstName,
		LastName:  opResp.LastName,
		Email:     newUserFull.PrimaryEmail,
		CompanyID: company.ID,
	}

	err = dbmap.Insert(&newUser)
	if err != nil {
		ErrorLog.Println("err inserting new  user:", userCXN.Credentials.Username, " csn: ", userCXN.Credentials.CompanyShortname, " err: ", err)
		return
	}
}
