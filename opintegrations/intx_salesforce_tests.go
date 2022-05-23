package main

import "time"

func sfCreateUserTest() {
	accessToken := "00D3F000000CtLr!ARoAQHBVJEbhxhdfX.ipxs.p6tDnOoAmbD0IP0Gaq_2HfJBSIW92cbMwIDKkN2M95jlOU3_wU5P3O7aeRqUPiUh1w31UXBdt"
	orgURL := "https://employernet--jared.cs92.my.salesforce.com"

	sfUserReq := SFCreateUserRequest{
		Email:             "test@test.com",
		FirstName:         "test1",
		LastName:          "tester",
		Username:          "test@tewrf3fwfwst.com",
		Alias:             "tester",
		CommunityNickname: "tester",
		TimeZoneSidKey:    "America/Los_Angeles", //
		LocaleSidKey:      "en_US",
		EmailEncodingKey:  "UTF-8",              //
		ProfileId:         "00e40000000yoveAAA", //
		LanguageLocaleKey: "en_US",
		IsActive:          true,
	}

	_, err := sfCreateUser(accessToken, orgURL, sfUserReq)
	if err != nil {
		ErrorLog.Println(err)
	}
}

func sfInactivateUserTest() {
	accessToken := "00D3F000000CtLr!ARoAQHBVJEbhxhdfX.ipxs.p6tDnOoAmbD0IP0Gaq_2HfJBSIW92cbMwIDKkN2M95jlOU3_wU5P3O7aeRqUPiUh1w31UXBdt"
	orgURL := "https://employernet--jared.cs92.my.salesforce.com"
	sfUserEmail := "test@test.com"

	sfQueryStr := `SELECT+Id+FROM+User+WHERE+Email='` + sfUserEmail + `'`
	sfResponse, err := sfQuery(accessToken, orgURL, sfQueryStr)
	if err != nil {
		ErrorLog.Println(err)
		return
	}

	id := ""
	ok := false
	if len(sfResponse.Records) > 0 {
		user := sfResponse.Records[0]
		id, ok = user["Id"].(string)
		if !ok {
			ErrorLog.Println("User not found")
			return
		}
	} else {
		ErrorLog.Println("User not found")
		return
	}

	err = sfInactivateUser(accessToken, orgURL, id)
	if err != nil {
		ErrorLog.Println(err)
	}
}

func sfTestQuery(accessToken string) {
	orgURL := "https://employernet--jared.cs92.my.salesforce.com"

	sfQueryStr := `SELECT+Id,+FirstName,+ProfileId,+TimeZoneSidKey+FROM+User+WHERE+Profile.Name='Standard%20User'+LIMIT+1`

	sfResponse, err := sfQuery(accessToken, orgURL, sfQueryStr)
	if err != nil {
		ErrorLog.Println(err)
		return
	}

	InfoLog.Printf("%+v\n", sfResponse)
}

func sfTestConstructNewUser() {
	orgURL := "https://employernet--jared.cs92.my.salesforce.com"
	var opAcctId int64 = 48287249
	var opCompId int64 = 33560858

	thisCompanyIntegration, err := getCompanyIntegrationByIntegrationString("salesforce", 1)
	if err != nil {
		ErrorLog.Println("salesforce couldnt look up company integration: ", err)
		return
	}

	accessToken, err := getAccessToken(thisCompanyIntegration)
	if err != nil {
		ErrorLog.Println("salesforce ADD getAccessToken err: ", err)
		return
	}

	cxn := chooseOPAPICxn(opCompId)
	opUsr, err := cxn.GetFullEmployeeInfo(opAcctId, opCompId)
	if err != nil {
		ErrorLog.Println("getFullEmployeeInfo err: ", err)
		return
	}

	newUsr, err := constructNewSalesforceUser(accessToken, orgURL, opCompId, opUsr, thisCompanyIntegration, []HFCompanyIntegrationResultJoin{})
	if err != nil {
		ErrorLog.Println("constructNewSalesforceUser err: ", err)
		return
	}

	// InfoLog.Printf("%+v\n", newUsr)

	_, err = sfCreateUser(accessToken, orgURL, newUsr)
	if err != nil {
		ErrorLog.Println("sfCreateUser err: ", err)
		return
	}

	InfoLog.Printf("WORKED!")
}

func sfHiredTest() {
	accountID := 48287283
	opCompanyID := 33560858
	companyID := 1

	fname := "Demetrius"
	lname := "Borba"
	email := "jaredshahbazian@gmail.com"

	falseV := false
	newHiredEvent := HFEvent{
		Type:        HIRED_TYPE_ID,
		OPAccountID: int64(accountID),
		EventDate:   "03/22/2018",
		CompanyID:   int64(companyID),
		FirstName:   fname,
		LastName:    lname,
		Email:       email,
		Processed:   &falseV,
		Created:     time.Now().Unix(),
	}

	dbmap.Insert(&newHiredEvent)

	cxn := chooseOPAPICxn(int64(opCompanyID))
	employeeRecord, err := cxn.GetFullEmployeeInfo(int64(accountID), int64(opCompanyID))
	if err != nil {
		ErrorLog.Println(err)
		return
	}

	sfHired(&newHiredEvent, int64(opCompanyID), employeeRecord, []HFCompanyIntegrationResultJoin{})
}

func sfFiredTest() {
	accountID := 48287283
	companyID := 1

	fname := "Demetrius"
	lname := "Borba"
	email := "jaredshahbazian@gmail.com"

	falseV := false
	newHiredEvent := HFEvent{
		Type:        FIRED_TYPE_ID,
		OPAccountID: int64(accountID),
		EventDate:   "03/22/2018",
		CompanyID:   int64(companyID),
		FirstName:   fname,
		LastName:    lname,
		Email:       email,
		Processed:   &falseV,
		Created:     time.Now().Unix(),
	}

	dbmap.Insert(&newHiredEvent)

	// sfFired(&newHiredEvent)
}
