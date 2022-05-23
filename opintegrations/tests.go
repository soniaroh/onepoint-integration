package main

import (
	"fmt"
	"time"

	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

func simulateHiring(hiredTrueFiredFalse bool, accountID int64) {
	companyID := 1

	cxn := chooseOPAPICxn(int64(companyID))
	emp, err := cxn.GetFullEmployeeInfo(accountID, int64(33560858))
	if err != nil {
		ErrorLog.Println("simulateHiring could not lookup employee: ", err)
		return
	}

	typeX := HIRED_TYPE_ID
	if !hiredTrueFiredFalse {
		typeX = FIRED_TYPE_ID
	}

	falseV := false
	newHiredEvent := HFEvent{
		Type:        int8(typeX),
		OPAccountID: int64(accountID),
		EventDate:   time.Now().Format("01/02/2006"),
		CompanyID:   int64(companyID),
		FirstName:   emp.FirstName,
		LastName:    emp.LastName,
		Email:       emp.PrimaryEmail,
		Processed:   &falseV,
		Created:     time.Now().Unix(),
	}

	err = dbmap.Insert(&newHiredEvent)
	if err != nil {
		ErrorLog.Println("-- couldnt Insert new event: ", err)
		return
	}

	lookupHFCIsAndRun(&newHiredEvent)
}

func sendTestEmail() {
	emailHeaderInfo := sgEmailFields{
		From:    &mail.Email{Name: "OnePoint Connect", Address: passwords.NO_REPLY_EMAILER_ADDRESS},
		To:      []*mail.Email{&mail.Email{Address: passwords.ADMIN_NOTIFICATION_EMAIL_ADDRESS}, &mail.Email{Address: "jaredshahbazian@gmail.com"}},
		Subject: "Action Required: Complete Your Application with TEST COMPANY",
	}

	body := ApplicationNotificationEmailBody{
		ApplicantName: "Jared Shahbazian",
		JobTitle:      "President",
		Source:        "Indeed",
	}

	err := sendTemplatedEmailSendGrid(emailHeaderInfo, JOB_APPLICATION_EMAIL_TEMPLATE, body)
	if err != nil {
		ErrorLog.Println("email test err: ", err)
	}
}

func testOPUpdateEmail() {
	const accountID int64 = 48989011
	const companyID int64 = 33560858

	cxn := chooseOPAPICxn(companyID)
	emp, err := cxn.GetFullEmployeeInfo(accountID, companyID)
	if err != nil {
		ErrorLog.Println("couldnt find emp: ", err)
		return
	}

	err = sendNewEmailToOP(1, emp, "jaredshahbazian@gmail.com")
	if err != nil {
		ErrorLog.Println("sendNewEmailToOP failed: ", err)
	} else {
		InfoLog.Println("sendNewEmailToOP worked !!")
	}
}

func testCCFilter() {
	cxn := chooseOPAPICxn(33560858)

	fullEmp, _ := cxn.GetFullEmployeeInfo(48287263, 33560858)

	coint, _ := getCompanyIntegrationByIntegrationString("gsuite", 1)

	blocked, err := checkHFCostCenterBlocked(coint, fullEmp)
	if err != nil {
		ErrorLog.Println(err)
		return
	}
	InfoLog.Println(blocked)
}

func testHFSummaryEmail(hiredTrueFiredFalse, includeSuccess, includeFail bool) {
	//func sendHFSummaryEmail(
	//+ productSettings map[string]interface{},
	//+ fullEmployeeRecord opapi.OPFullEmployeeRecord,
	//+ newHFEvent *HFEvent,
	//allResults []HFCompanyIntegrationResultJoin) {

	const employeeid = 48287363
	const coshortname = "ZJARED"
	const producturl = "user-provisioning"

	company, _ := lookupCompanyByShortname(coshortname)
	cp, err := getCompanyProductByURL(company.ID, producturl)
	if err != nil {
		ErrorLog.Println("testHFSummaryEmail qutting getCompanyProductByURL err: ", err.Error())
		return
	}

	cxn := chooseOPAPICxn(company.OPID)
	emp, err := cxn.GetFullEmployeeInfo(employeeid, company.OPID)
	if err != nil {
		ErrorLog.Println("testHFSummaryEmail qutting GetFullEmployeeInfo err: ", err.Error())
		return
	}

	hfEvent := HFEvent{
		ID:          1,
		OPAccountID: employeeid,
		EventDate:   "12/12/2018",
		CompanyID:   company.ID,
		FirstName:   emp.FirstName,
		LastName:    emp.LastName,
		Email:       emp.PrimaryEmail,
		Created:     time.Now().Unix(),
	}

	switch hiredTrueFiredFalse {
	case true:
		hfEvent.Type = HIRED_TYPE_ID
	case false:
		hfEvent.Type = FIRED_TYPE_ID
	}

	allResults := []HFCompanyIntegrationResultJoin{}

	if includeSuccess {
		ci, _ := getCompanyIntegrationByIntegrationString("gsuite", company.ID)
		result := HFResult{
			Description: "Success description Success description Success description Success description .",
			Status:      HF_SUCCESS_LABEL,
		}
		creds := HFCreatedCredentials{
			Username: "username",
			Password: "password",
		}

		resultjoin := HFCompanyIntegrationResultJoin{
			CompanyIntegration:   transformCIToPublic(ci),
			Result:               result,
			GeneratedCredentials: creds,
		}

		allResults = append(allResults, resultjoin)
	}

	if includeFail {
		ci, _ := getCompanyIntegrationByIntegrationString("box", company.ID)
		result := HFResult{
			Description: "Fail description Fail description Fail description Fail description .",
			Status:      HF_FAILED_LABEL,
		}

		resultjoin := HFCompanyIntegrationResultJoin{
			CompanyIntegration: transformCIToPublic(ci),
			Result:             result,
		}

		allResults = append(allResults, resultjoin)
	}

	sendHFSummaryEmail(cp.Settings, emp, &hfEvent, allResults)
}

func testGoogleHireAdminEmail() {
	creationLog := &GoogleHireCreatedApplicants{
		CompanyID:               1,
		GHApplicantResourceName: "whuwihfiuheiwef",
		OPApplicantUsername:     "testcandidate@gmail.com",
		ApplicantName:           "Test Candidate",
		ApplicantEmail:          "testcandidate@gmail.com",
		CreatedAt:               time.Now().UTC().Unix(),
	}

	creationLog.SendCompanyNotifications()
}

func testGoogleHireApplicantEmail() {
	creationLog := &GoogleHireCreatedApplicants{
		CompanyID:               1,
		GHApplicantResourceName: "whuwihfiuheiwef",
		OPApplicantUsername:     "jared.shahbazian@onehcm.com",
		ApplicantName:           "Test Candidate",
		ApplicantEmail:          "jared.shahbazian@onehcm.com",
		CreatedAt:               time.Now().UTC().Unix(),
	}

	loginLink := fmt.Sprintf("https://secure.onehcm.com/ta/%s.login", "ZJARED")

	creationLog.SendApplicantNotifications("P@ssw0rd", loginLink)
}
