package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	sgmail "github.com/sendgrid/sendgrid-go/helpers/mail"
)

type BackgroundCheck struct {
	ID            int64  `db:"id, primarykey, autoincrement" json:"id"`
	CompanyID     int64  `db:"company_id" json:"company_id"`
	OPApplicantID string `db:"op_applicant_id" json:"op_applicant_id"`
	ClientEmail   string `db:"client_email" json:"client_email"`
	OPJobID       string `db:"op_job_id" json:"op_job_id"`
	FirstName     string `db:"first_name" json:"first_name"`
	LastName      string `db:"last_name" json:"last_name"`
	Email         string `db:"email" json:"email"`
	ProviderID    string `db:"provider_id" json:"provider_id"`
	Type          string `db:"type" json:"type"`
	Status        string `db:"status" json:"status"`
	FinishedURL   string `db:"finished_url" json:"finished_url"`
	Initiated     int64  `db:"initiated" json:"initiated"`
	LastUpdated   *int64 `db:"last_updated" json:"last_updated"`
}

type BGCHECKReport struct {
	ApplicantID             string `csv:"Applicant Id"`
	LastName                string `csv:"Last Name"`
	FirstName               string `csv:"First Name"`
	MiddleInitial           string `csv:"MI"`
	Email                   string `csv:"Email"`
	SS                      string `csv:"SS#"`
	ApplicantionHiringStage string `csv:"Job Application Hiring Stage"`
	CellPhone               string `csv:"Cell Phone"`
	HomePhone               string `csv:"Home Phone"`
	WorkPhone               string `csv:"Work Phone"`
	FullAddress             string `csv:"Full Address"`
	RequisitionNumber       string `csv:"Requisition #"`
	ContactEmail            string `csv:"AssureHire User Email to Use"`
	BackgroundCheckLevel    string `csv:"Background Check Level"`
}

type AssureHireNewBGCPost struct {
	Email       string `json:"email"`
	PackageName string `json:"package_id"`
	Address     string `json:"address"`
	SS          string `json:"ssn"`
	HomePhone   string `json:"home_phone"`
	MobilePhone string `json:"mobile_phone"`
	LastName    string `json:"last_name"`
	FirstName   string `json:"first_name"`
}

func registerBgCheckRoutes(router *gin.Engine) {
	router.POST("/api/background_checks/providers/:provider", bgCheckWebhookHandler)
}

const (
	BGC_PROD_COMPANY_SHORT    = "MARIANI"
	BGC_TEST_COMPANY_SHORT    = "ZJARED"
	BGC_PROD_REPORT_ID        = "37617485"
	BGC_TEST_REPORT_ID        = "37617483"
	USE_ZJARED                = false
	SEND_TO_AH                = true
	USE_TEST_EMAIL            = false
	USE_TEST_SSN              = false
	ASSURE_HIRE_API_SUBDOMAIN = ""
	TEST_SSN                  = "773111571"
)

// For now, we just do Mariani x AssureHire
func runBgChecks() {
	comp, err := lookupCompanyByShortname("MARIANI")
	if err != nil {
		ErrorLog.Println("could not lookup mariani in bgCheckWebhookHandler")
		return
	}

	applicants, err := getBGCApplicants()
	if err != nil {
		ErrorLog.Println("getBGCApplicants err: " + err.Error())
		return
	}

	InfoLog.Printf("runBgChecks report has %d applications in the BC stage\n", len(applicants))

	alreadyCreatedCount := 0
	missingInfoCount := 0
	triedToCreateCount := 0
	for _, applicant := range applicants {
		if applicant.BackgroundCheckLevel == "DO NOT PERFORM BC" || applicant.BackgroundCheckLevel == "" {
			ErrorLog.Printf("Mariani BGC applicant MISSING INFO - applicantID: %s, but BC Level Custom Field is: %s\n", applicant.ApplicantID, applicant.BackgroundCheckLevel)
			missingInfoCount++
			continue
		}

		if applicant.ContactEmail == "" {
			ErrorLog.Printf("Mariani BGC applicant MISSING INFO - applicantID: %s, but AssureHire Contact email is blank\n", applicant.ApplicantID)
			missingInfoCount++
			continue
		}

		// - Need either email or SSN + F Name L Name or will be ignored
		if !((applicant.Email != "") || (applicant.SS != "" && applicant.FirstName != "" && applicant.LastName != "")) {
			ErrorLog.Printf("Mariani BGC applicant MISSING INFO - applicantID: %s, Email: %s, SS: %s, FN: %s, ln: %s\n", applicant.ApplicantID, applicant.Email, applicant.SS, applicant.FirstName, applicant.LastName)
			missingInfoCount++
			continue
		}

		existingBGCs := []BackgroundCheck{}
		_, err = dbmap.Select(&existingBGCs, "SELECT * FROM background_checks WHERE op_applicant_id = ? OR email = ?", applicant.ApplicantID, applicant.Email)
		if len(existingBGCs) > 0 {
			alreadyCreatedCount++
			continue
		}

		InfoLog.Printf("Mariani BGC - new BackgroundCheck to create, email: %s, level: %s\n", applicant.ContactEmail, applicant.BackgroundCheckLevel)
		triedToCreateCount++

		newBGC, err := sendNewBackgroundCheck(comp, applicant)
		if err != nil {
			ErrorLog.Println("sendNewBackgroundCheck Assure hire err: ", err)
			continue
		}

		go sendBGCNotificationEmail(newBGC)

		err = dbmap.Insert(&newBGC)
		if err != nil {
			ErrorLog.Println("sendNewBackgroundCheck Assure hire Insert err: ", err)
			continue
		}
	}

	InfoLog.Printf("runBgChecks done, missing info: %d, already started: %d, sent to AH to start: %d", missingInfoCount, alreadyCreatedCount, triedToCreateCount)
}

func sendNewBackgroundCheck(company Company, bgcReportApplicant BGCHECKReport) (BackgroundCheck, error) {
	newBGC := BackgroundCheck{}

	ssn := bgcReportApplicant.SS
	if USE_TEST_SSN {
		ssn = TEST_SSN
	}

	if SEND_TO_AH {
		sendingBody := AssureHireNewBGCPost{
			Email:       bgcReportApplicant.Email,
			PackageName: bgcReportApplicant.BackgroundCheckLevel,
			Address:     bgcReportApplicant.FullAddress,
			SS:          ssn,
			HomePhone:   bgcReportApplicant.HomePhone,
			MobilePhone: bgcReportApplicant.CellPhone,
			FirstName:   bgcReportApplicant.FirstName,
			LastName:    bgcReportApplicant.LastName,
		}

		url := fmt.Sprintf("https://%sassurehire.com/api/v1/backgroundchecks", ASSURE_HIRE_API_SUBDOMAIN)

		b := new(bytes.Buffer)
		json.NewEncoder(b).Encode(sendingBody)

		req, err := http.NewRequest("POST", url, b)
		if err != nil {
			return newBGC, errors.New("sendNewBackgroundCheck NewRequest err: " + err.Error())
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/json")

		authUser := bgcReportApplicant.ContactEmail
		if USE_TEST_EMAIL {
			authUser = "robin@marianinut.com"
		}

		// TODO: what if missing? will error
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s:%s", authUser, passwords.ASSURE_HIRE_TOKEN))

		InfoLog.Printf("sending to AH - body: %+v, url: %s, w authUser: %s\n", sendingBody, url, authUser)

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return newBGC, errors.New("sendNewBackgroundCheck Do err: " + err.Error())
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 300 {
			bodyBytes, _ := ioutil.ReadAll(resp.Body)
			bodyString := string(bodyBytes)

			return newBGC, errors.New(fmt.Sprintf("sendNewBackgroundCheck bad request- status: %d, body: %s", resp.StatusCode, bodyString))
		}

		assurehireResp := AssureHireBGC{}
		err = json.NewDecoder(resp.Body).Decode(&assurehireResp)
		if err != nil {
			bodyBytes, _ := ioutil.ReadAll(resp.Body)
			bodyString := string(bodyBytes)

			ErrorLog.Println("AssureHire sent body: ", bodyString)
			return newBGC, errors.New("sendNewBackgroundCheck NewDecoder err: " + err.Error())
		}

		newBGC.ClientEmail = authUser

		newBGC.CompanyID = company.ID
		newBGC.Email = bgcReportApplicant.Email
		newBGC.FirstName = bgcReportApplicant.FirstName
		newBGC.Initiated = time.Now().Unix()
		newBGC.LastName = bgcReportApplicant.LastName
		newBGC.OPApplicantID = bgcReportApplicant.ApplicantID
		newBGC.OPJobID = bgcReportApplicant.RequisitionNumber
		newBGC.ProviderID = assurehireResp.ID
		newBGC.Status = assurehireResp.Status
		newBGC.Type = bgcReportApplicant.BackgroundCheckLevel
		newBGC.FinishedURL = assurehireResp.ReportURL
	} else {
		newBGC.ClientEmail = bgcReportApplicant.ContactEmail
		newBGC.CompanyID = company.ID
		newBGC.Email = bgcReportApplicant.Email
		newBGC.FirstName = bgcReportApplicant.FirstName
		newBGC.Initiated = time.Now().Unix()
		newBGC.LastName = bgcReportApplicant.LastName
		newBGC.OPApplicantID = bgcReportApplicant.ApplicantID
		newBGC.OPJobID = bgcReportApplicant.RequisitionNumber
		newBGC.ProviderID = strconv.Itoa(int(time.Now().Unix()))
		newBGC.Status = "test bgc"
		newBGC.Type = bgcReportApplicant.BackgroundCheckLevel
		newBGC.FinishedURL = "test.com"
	}

	return newBGC, nil
}

func sendBGCNotificationEmail(newBGC BackgroundCheck) {
	emailHeaderInfo := sgEmailFields{
		Subject: fmt.Sprintf("New Background Check initiated for %s %s", newBGC.FirstName, newBGC.LastName),
		From:    &sgmail.Email{Name: "OnePoint HCM", Address: passwords.NO_REPLY_EMAILER_ADDRESS},
		To:      []*sgmail.Email{&sgmail.Email{Address: newBGC.ClientEmail}},
	}

	emailBody := NewBGCNotificationBody{
		ApplicantName: newBGC.FirstName + " " + newBGC.LastName,
	}

	err := sendTemplatedEmailSendGrid(emailHeaderInfo, BGC_NOTIFICATION_TEMPLATE, emailBody)
	if err != nil {
		ErrorLog.Printf("sendBGCNotificationEmail emailing err: %v\n", err)
		return
	}

	InfoLog.Println("New AssureHire BGC email sent successfully")
}

func getBGCApplicants() ([]BGCHECKReport, error) {
	shortname := BGC_TEST_COMPANY_SHORT
	reportID := BGC_TEST_REPORT_ID
	if !USE_ZJARED {
		shortname = BGC_PROD_COMPANY_SHORT
		reportID = BGC_PROD_REPORT_ID
	}

	bgcRows := []BGCHECKReport{}

	comp, err := lookupCompanyByShortname(shortname)
	if err != nil {
		return bgcRows, errors.New("could not find MARIANI: " + err.Error())
	}

	cxn := chooseOPAPICxn(comp.OPID)

	maxTries := 3
	success := false
	reportURL := fmt.Sprintf("https://secure.onehcm.com/ta/rest/v1/report/%s/%s?company:shortname=%s", "saved", reportID, shortname)
	tries := 0

	// retries bc reports fail
	for {
		if success {
			break
		}

		if tries == maxTries {
			return bgcRows, errors.New("runBgChecks err! " + err.Error())
		}

		err = cxn.GenricReport(reportURL, &bgcRows)
		tries++

		if err != nil {
			continue
		}

		success = true
	}

	return bgcRows, nil
}

func bgCheckWebhookHandler(c *gin.Context) {
	envParam := c.Query("env")

	input := make(map[string]interface{})
	if err := c.ShouldBindWith(&input, binding.JSON); err != nil {
		ErrorLog.Println("err binding bgCheckWebhookHandler json: ", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An error occured on our side"})
		return
	}

	InfoLog.Printf("new assurehire webhook, env: %s, data: %+v\n", envParam, input)

	inputData, ok := input["data"].(map[string]interface{})
	if !ok {
		ErrorLog.Println("bgCheckWebhookHandler err input[data] cannot type cast to map[string]interface{}")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An error occured on our side"})
		return
	}

	inputObject, ok := inputData["object"].(map[string]interface{})
	if !ok {
		ErrorLog.Println("bgCheckWebhookHandler err input[data] cannot type cast to map[string]interface{}")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An error occured on our side"})
		return
	}

	bgcID, ok := inputObject["id"].(string)
	if !ok {
		ErrorLog.Println("bgCheckWebhookHandler cannot get ID")
		c.JSON(http.StatusNotFound, gin.H{"error": "We do not have that background check in our records"})
		return
	}

	thisBGC := BackgroundCheck{}
	err := dbmap.SelectOne(&thisBGC, "SELECT * FROM background_checks WHERE provider_id = ?", bgcID)
	if err != nil {
		ErrorLog.Printf("unable to lookup bgc in webhook: id - %s\n", bgcID)
		c.JSON(http.StatusNotFound, gin.H{"error": "We do not have that background check in our records"})
		return
	}

	shouldUpdate := false

	status := thisBGC.Status
	bgcStatus, ok := inputObject["status"].(string)
	if !ok {
		ErrorLog.Println("bgCheckWebhookHandler cannot get bgcStatus")
	} else {
		status = bgcStatus
		shouldUpdate = true
	}

	url := thisBGC.FinishedURL
	bgcReportURL, ok := inputObject["report_url"].(string)
	if !ok {
		ErrorLog.Println("bgCheckWebhookHandler cannot get bgcReportURL")
	} else {
		url = bgcReportURL
		shouldUpdate = true
	}

	if !shouldUpdate {
		ErrorLog.Println("bgCheckWebhookHandler no need to update")
		c.JSON(http.StatusOK, gin.H{"msg": "OK"})
		return
	}

	nowSecs := time.Now().Unix()
	thisBGC.Status = status
	thisBGC.FinishedURL = url
	thisBGC.LastUpdated = &nowSecs

	_, err = dbmap.Update(&thisBGC)
	if err != nil {
		ErrorLog.Printf("unable to Update bgc in webhook: id - %s\n", bgcID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An error occured on our side"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Thanks"})
}

type AssureHireBGC struct {
	ID                  string      `json:"id"`
	CreatedAt           string      `json:"created_at"`
	SubmittedAt         string      `json:"submitted_at"`
	UpdatedAt           string      `json:"updated_at"`
	CompletedAt         string      `json:"completed_at"`
	DueAt               string      `json:"due_at"`
	Turnaround          int         `json:"turnaround"`
	Status              string      `json:"status"`
	StatusMsg           string      `json:"status_msg"`
	Score               string      `json:"score"`
	CopyRequested       bool        `json:"copy_requested"`
	Purpose             string      `json:"purpose"`
	BillCode            string      `json:"bill_code"`
	JobCode             string      `json:"job_code"`
	PackageID           string      `json:"package_id"`
	ReportURL           string      `json:"report_url"`
	ConsentURL          string      `json:"consent_url"`
	ApplicantURL        string      `json:"applicant_url"`
	LastName            string      `json:"last_name"`
	FirstName           string      `json:"first_name"`
	MiddleName          interface{} `json:"middle_name"`
	NameSuffix          interface{} `json:"name_suffix"`
	Ssn                 string      `json:"ssn"`
	Dob                 string      `json:"dob"`
	DriverLicenseNumber string      `json:"driver_license_number"`
	DriverLicenseState  string      `json:"driver_license_state"`
	Gender              interface{} `json:"gender"`
	Title               interface{} `json:"title"`
	Email               string      `json:"email"`
	MobilePhone         string      `json:"mobile_phone"`
	HomePhone           string      `json:"home_phone"`
	WorkPhone           interface{} `json:"work_phone"`
	AddressStreet       string      `json:"address_street"`
	AddressCity         string      `json:"address_city"`
	AddressState        string      `json:"address_state"`
	AddressZip          string      `json:"address_zip"`
	AccessFee           interface{} `json:"access_fee"`
	Price               string      `json:"price"`
	Meta                struct {
	} `json:"meta"`
	AddressHistory []struct {
		Source        string      `json:"source"`
		FromDate      string      `json:"from_date"`
		ToDate        string      `json:"to_date"`
		FirstName     string      `json:"first_name"`
		MiddleName    string      `json:"middle_name"`
		LastName      string      `json:"last_name"`
		NameSuffix    string      `json:"name_suffix"`
		AddressCounty interface{} `json:"address_county"`
		AddressStreet string      `json:"address_street"`
		AddressCity   string      `json:"address_city"`
		AddressState  string      `json:"address_state"`
		AddressZip    string      `json:"address_zip"`
	} `json:"address_history"`
	Products []struct {
		ID                 string      `json:"id"`
		SubmittedAt        string      `json:"submitted_at"`
		UpdatedAt          string      `json:"updated_at"`
		CompletedAt        string      `json:"completed_at"`
		DueAt              string      `json:"due_at"`
		ProductID          string      `json:"product_id"`
		ProductDescription string      `json:"product_description"`
		Status             string      `json:"status"`
		PackageID          string      `json:"package_id"`
		AccessFee          interface{} `json:"access_fee"`
		Price              interface{} `json:"price"`
		Request            struct {
			LicenseNum    string `json:"license_num"`
			IssuingAgency string `json:"issuing_agency"`
			Issued        string `json:"issued"`
			Expires       string `json:"expires"`
		} `json:"request"`
	} `json:"products"`
}
