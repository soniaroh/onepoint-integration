package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"opapi"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

type JobApplication struct {
	ID                   int64       `db:"id, primarykey, autoincrement" json:"id"`
	CompanyID            int64       `db:"company_id" json:"company_id"`
	OPJobID              string      `db:"op_job_id" json:"op_job_id"`
	Source               string      `db:"source" json:"source"`
	SourceApplicationID  string      `db:"source_application_id" json:"source_application_id"`
	AppliedOnMillis      int64       `db:"applied_on_millis" json:"applied_on_millis"`
	ApplicantName        string      `db:"applicant_name" json:"applicant_name"`
	ApplicantEmail       string      `db:"applicant_email" json:"applicant_email"`
	ApplicantPhoneNumber string      `db:"applicant_phone_number" json:"applicant_phone_number"`
	FinishEmailSent      *bool       `db:"finish_email_sent" json:"finish_email_sent"`
	FollowUpEmailSent    *bool       `db:"followup_email_sent" json:"followup_email_sent"`
	AppliedOnOP          *bool       `db:"applied_on_op" json:"applied_on_op"`
	UserReviewed         *bool       `db:"user_reviewed" json:"user_reviewed"`
	ResumeURL            *string     `db:"resume_url" json:"resume_url"`
	Data                 PropertyMap `db:"data,size:10000" json:"data"`

	OPApplicantID *int64 `db:"op_applicant_id" json:"op_applicant_id"`

	ApplicantFirstName string `db:"-" json:"applicant_first_name"`
	ApplicantLastName  string `db:"-" json:"applicant_last_name"`
}

type JobSiteApplicationData interface {
	ConvertToOPApplicant() *opapi.Applicant
	GetResumeFileAndExtension() (*string, string)
}

type JobReqPublic struct {
	OPID              string                  `json:"id"`
	Title             string                  `json:"title"`
	ApplicationCount  int64                   `json:"application_count"`
	Applications      []JobApplication        `json:"applications"`
	IndeedSponsorship IndeedSponsorshipObject `json:"indeed_sponsorship"`
	ZRSponsorship     ZRSponsorshipObject     `json:"ziprecruiter_sponsorship"`
}

const (
	NEW_APPLICANT_PROCESS = false

	JOB_APPS_COUNT         = 10
	JOB_RESUMES_BUCKETNAME = "onepoint-resumes"
	RESUMES_URL            = "https://storage.googleapis.com/onepoint-resumes/"
)

func registerJobsRoutes(router *gin.Engine) {
	router.GET("/api/recruitment/applications", getJobApplicationsHandler)
	router.GET("/api/recruitment/jobs/:opJobID/:appPage", getJobHandler)
	router.POST("/api/recruitment/applications/:appID/sendFinishEmail", sendFinishEmailHandler)
	router.POST("/api/recruitment/applications/:appID/markAsReviewed", markAsReviewedHandler)
	router.POST("/api/recruitment/application", jobApplicationPostHandler)
	router.POST("/api/recruitment/zipsponsorship/:opJobID", zipSponsorshipPostHandler)
	router.POST("/api/recruitment/indeedsponsorship/:opJobID", indeedSponsorshipPostHandler)
	router.POST("/api/recruitment/indeedsponsorship/:opJobID/:sponsorshipID/update", indeedSponsorshipUpdateHandler)
	router.GET("/api/recruitment/startprocess/:processName", startRecruitmentProcessHandler)
}

func startRecruitmentProcessHandler(c *gin.Context) {
	err := isAdminUser(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	processName := c.Param("processName")
	if processName == "" {
		ErrorLog.Println("blank processName path parameter")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	switch processName {
	case "runFollowUps":
		startFollowUpsHandler(c)
		return
	case "conversionReport":
		getRecruitmentReportsHandler(c)
		return
	case "runJobFeeds":
		go runIndeedFeed()
		c.JSON(200, nil)
		return
	}
}

func startFollowUpsHandler(c *gin.Context) {
	err := isAdminUser(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	companiesVar := c.Query("companies")

	if companiesVar == "all" {
		go runApplyFollowUps()
	} else {
		co, err := lookupCompanyByShortname(companiesVar)
		if err != nil {
			ErrorLog.Println("startFollowUpsHandler get company err: " + err.Error())
			c.JSON(404, gin.H{"error": err.Error()})
			return
		}

		err = updateAppliedStatus(co)
		if err != nil {
			ErrorLog.Println("startFollowUpsHandler updateAppliedStatus err: " + err.Error())
			c.JSON(404, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(200, nil)
}

func getJobHandler(c *gin.Context) {
	thisCompany, err := lookupCompany(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	opJobID := c.Param("opJobID")
	if opJobID == "" {
		ErrorLog.Println("blank opJobID path parameter")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	page := 1
	appPage := c.Param("appPage")
	if appPage != "" {
		page64, err := strconv.ParseInt(appPage, 10, 64)
		if err != nil {
			ErrorLog.Println("appPage is not a number")
			c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
			return
		}

		page = int(page64)
	}

	job, err := getJobToReturn(thisCompany, opJobID, page)
	if err != nil {
		ErrorLog.Println("err getJobToReturn: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown error"})
		return
	}

	c.JSON(200, job)
	return
}

func getRecruitmentReportsHandler(c *gin.Context) {
	err := isAdminUser(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	overall, codata, err := runApplyStats()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	retBody := ApplyStatsReturnBody{
		OverallJBApplied:        overall.ApplyStats.JBApplied,
		OverallOPApplied:        overall.ApplyStats.OPApplied,
		OverallPercentConverted: overall.ApplyStats.PercentConverted,
		CoData:                  codata,
	}

	c.JSON(200, retBody)
	return
}

func getJobToReturn(thisCompany Company, opJobID string, page int) (JobReqPublic, error) {
	cxn := chooseOPAPICxn(thisCompany.OPID)
	jobReq, err := cxn.GetDetailedJobReqByID(thisCompany.ShortName, opJobID)
	if err != nil {
		ErrorLog.Println("GetDetailedJobReqByID err")
		return JobReqPublic{}, err
	}

	opJobIDStr := strconv.Itoa(jobReq.ID)

	jobAppCount, jobApps, err := findAllJobsAppsByOPID(opJobIDStr, page)
	if err != nil {
		return JobReqPublic{}, errors.New("findAllJobsAppsByOPID err: " + err.Error())
	}

	IndeedSponsorship, err := findAllIndeedSponsorshipByOPID(opJobIDStr)
	if err != nil {
		return JobReqPublic{}, err
	}

	ZipRecruiterSponsorships, err := findAllZRSponsorshipByOPID(opJobIDStr)
	if err != nil {
		return JobReqPublic{}, err
	}

	thisJob := JobReqPublic{
		OPID:              opJobIDStr,
		Title:             jobReq.JobTitle,
		Applications:      jobApps,
		IndeedSponsorship: IndeedSponsorship,
		ZRSponsorship:     ZipRecruiterSponsorships,
		ApplicationCount:  jobAppCount,
	}

	return thisJob, nil
}

func markAsReviewedHandler(c *gin.Context) {
	thisCompany, err := lookupCompany(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	appID := c.Param("appID")
	if appID == "" {
		ErrorLog.Println("blank appID path parameter")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	thisApp := JobApplication{}
	err = dbmap.SelectOne(&thisApp, "SELECT * FROM job_applications WHERE id = ? AND company_id = ?", appID, thisCompany.ID)
	if err != nil {
		ErrorLog.Println("markAsReviewedHandler SelectOne: ", appID, thisCompany.ID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	trueV := true
	thisApp.UserReviewed = &trueV
	_, err = dbmap.Update(&thisApp)
	if err != nil {
		ErrorLog.Println("markAsReviewedHandler SelectOne: ", appID, thisCompany.ID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	c.JSON(200, thisApp)
	return
}

func sendFinishEmailHandler(c *gin.Context) {
	thisCompany, err := lookupCompany(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	appID := c.Param("appID")
	if appID == "" {
		ErrorLog.Println("blank appID path parameter")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	thisApp := JobApplication{}
	err = dbmap.SelectOne(&thisApp, "SELECT * FROM job_applications WHERE id = ? AND company_id = ?", appID, thisCompany.ID)
	if err != nil {
		ErrorLog.Println("sendFinishEmailHandler SelectOne: ", appID, thisCompany.ID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	err = sendFinishEmailAndUpdateSent(&thisApp, thisCompany)
	if err != nil {
		ErrorLog.Println("sendFinishEmail err: " + err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"error": "An error occured"})
		return
	}

	c.JSON(200, thisApp)
	return
}

func sendFinishEmailAndUpdateSent(thisApp *JobApplication, thisCompany Company) error {
	// TODO: check if theyve applied on OP!

	err := sendJobAppFinishEmail(thisApp, thisCompany, false)
	if err != nil {
		falseV := false
		thisApp.FinishEmailSent = &falseV
		_, uerr := dbmap.Update(thisApp)
		if uerr != nil {
			return errors.New("sendJobAppFinishEmail err and updating record err: " + uerr.Error())
		}
		return errors.New("sendJobAppFinishEmail err: " + err.Error())
	}

	trueV := true
	thisApp.FinishEmailSent = &trueV
	_, err = dbmap.Update(thisApp)

	return err
}

func sendJobAppFinishEmail(jobApp *JobApplication, company Company, reminder bool) error {
	cxn := chooseOPAPICxn(company.OPID)

	maxTries := 2
	success := false
	tries := 0

	var err error
	jobReqDetail := opapi.JobReqDetail{}

	for {
		if success {
			break
		}

		if tries == maxTries {
			return errors.New("GetDetailedJobReqByID max tries err!")
		}

		jobReqDetail, err = cxn.GetDetailedJobReqByID(company.ShortName, jobApp.OPJobID)
		tries++

		if err != nil {
			if err.Error() == "Job Requisition is no longer active." {
				return errors.New("Job Requisition is no longer active.")
			}

			continue
		}

		success = true
	}

	subject := fmt.Sprintf("ACTION REQUIRED: You have not completed your job application with %s", company.Name)
	if reminder {
		subject = "REMINDER: You have not completed your application with " + company.Name
	}

	emailCat := "apply-invite"
	if reminder {
		emailCat = "apply-reminder"
	}

	emailHeaderInfo := sgEmailFields{
		From:    &mail.Email{Name: "Recruitment Team", Address: passwords.NO_REPLY_EMAILER_ADDRESS},
		To:      []*mail.Email{&mail.Email{Address: jobApp.ApplicantEmail}},
		Subject: subject,
	}

	body := FinishApplicationEmailBody{
		ApplicantName: jobApp.ApplicantName,
		JobTitle:      jobReqDetail.JobTitle,
		CompanyName:   company.Name,
		ApplyURL:      fmt.Sprintf("https://secure.onehcm.com/ta/%s.jobs?ApplyToJob=%s&TrackId=%s", company.ShortName, jobApp.OPJobID, jobApp.Source),
	}

	template := JOB_FINISH_APP_EMAIL_TEMPLATE
	if reminder {
		template = JOB_FINISH_APP_REMINDER_EMAIL_TEMPLATE
	}

	err = sendTemplatedEmailSendGrid(emailHeaderInfo, template, body, emailCat)
	if err != nil {
		ErrorLog.Println("sendJobAppNotficationEmail email err: ", err)
		return err
	}

	return nil
}

func getJobApplicationsHandler(c *gin.Context) {
	thisCompany, err := lookupCompany(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	cxn := chooseOPAPICxn(thisCompany.OPID)
	jobReqs, err := fetchCompanyJobs(cxn, thisCompany.ShortName)
	if err != nil {
		ErrorLog.Printf("err fetching company jobs. company: %s, err: %v\n", thisCompany.ShortName, err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
		return
	}

	publicJobs := []JobReqPublic{}
	for _, jobReq := range jobReqs {
		jobAppCount, jobApps, _ := findAllJobsAppsByOPID(strconv.Itoa(jobReq.ID), 1)
		IndeedSponsorship, _ := findAllIndeedSponsorshipByOPID(strconv.Itoa(jobReq.ID))
		ZRSponsorship, _ := findAllZRSponsorshipByOPID(strconv.Itoa(jobReq.ID))

		thisJob := JobReqPublic{
			OPID:              strconv.Itoa(jobReq.ID),
			Title:             jobReq.JobTitle,
			Applications:      jobApps,
			IndeedSponsorship: IndeedSponsorship,
			ZRSponsorship:     ZRSponsorship,
			ApplicationCount:  jobAppCount,
		}

		publicJobs = append(publicJobs, thisJob)
	}

	c.JSON(200, publicJobs)
}

func (app *IndeedJobAppPost) ConvertToOPApplicant() *opapi.Applicant {
	wordsInName := strings.Split(app.Applicant.FullName, " ")
	fname, lname := "", ""
	if len(wordsInName) > 0 {
		fname = wordsInName[0]
		lname = wordsInName[len(wordsInName)-1]
	}

	opAccount := opapi.ApplicantAccount{
		Username:            app.Applicant.Email,
		FirstName:           fname,
		LastName:            lname,
		PrimaryEmail:        app.Applicant.Email,
		ForceChangePassword: true,
		Locked:              false,
	}

	var opPhone *opapi.ApplicantPhones
	if app.Applicant.PhoneNumber != "" {
		opPhone = &opapi.ApplicantPhones{
			CellPhone:      app.Applicant.PhoneNumber,
			PreferredPhone: "CELL",
		}
	}

	appAddress := &opapi.ApplicantAddress{Country: "USA"}

	newOPApplicant := &opapi.Applicant{
		Account: opAccount,
		Phones:  opPhone,
		Address: appAddress,
	}

	// location comes as "Sanger, CA" if given
	if app.Applicant.Resume.JSON.Location.City != "" {
		locationStrs := strings.Split(app.Applicant.Resume.JSON.Location.City, ",")
		if len(locationStrs) > 1 {
			newOPApplicant.Address.City = strings.TrimSpace(locationStrs[0])
			newOPApplicant.Address.State = strings.TrimSpace(locationStrs[1])
		}
	}

	if app.Applicant.Resume.JSON.Positions.Total > 0 {
		opPositions := []opapi.ApplicantWorkExperience{}
		for i, indeedPosition := range app.Applicant.Resume.JSON.Positions.Values {
			opPosition := opapi.ApplicantWorkExperience{
				Index: i + 1,
				Company: &opapi.ApplicantCompany{
					Name: indeedPosition.Company,
				},
			}

			locationStrs := strings.Split(indeedPosition.Location, ",")
			if len(locationStrs) > 1 {
				opPosition.Company.City = strings.TrimSpace(locationStrs[0])
				opPosition.Company.State = strings.TrimSpace(locationStrs[1])
			}

			title := opapi.ApplicantJobTitle{
				Index:          0,
				JobTitle:       indeedPosition.Title,
				JobDescription: indeedPosition.Description,
			}

			if indeedPosition.StartDateMonth != "-1" && indeedPosition.StartDateYear != "-1" {
				// dates need to be yyyy-mm-dd
				title.From = fmt.Sprintf("%s-%s-01", indeedPosition.StartDateYear, indeedPosition.StartDateMonth)
				if !indeedPosition.EndCurrent && indeedPosition.EndDateMonth != "-1" && indeedPosition.EndDateYear != "-1" {
					title.To = fmt.Sprintf("%s-%s-01", indeedPosition.EndDateYear, indeedPosition.EndDateMonth)
				}
			}

			opPosition.JobTitles = &[]opapi.ApplicantJobTitle{title}

			opPositions = append(opPositions, opPosition)
		}
		newOPApplicant.WorkExperiences = &opPositions
	}

	if app.Applicant.Resume.JSON.Educations.Total > 0 {
		schools := []opapi.ApplicantSchool{}
		for _, indeedSchool := range app.Applicant.Resume.JSON.Educations.Values {
			// Country is required by OP but location is not given by Indeed
			schools = append(schools, opapi.ApplicantSchool{SchoolName: indeedSchool.School, Country: "USA"})
		}
		newOPApplicant.Education = &opapi.ApplicantEducation{}
		newOPApplicant.Education.Schools = &schools
	}

	newOPApplicant.Summary = app.Applicant.Resume.JSON.Summary
	newOPApplicant.OtherSkills = app.Applicant.Resume.JSON.Skills

	return newOPApplicant
}

func (app *ZRJobAppPost) ConvertToOPApplicant() *opapi.Applicant {
	opAccount := opapi.ApplicantAccount{
		Username:            app.Email,
		FirstName:           app.FirstName,
		LastName:            app.LastName,
		PrimaryEmail:        app.Email,
		ForceChangePassword: true,
		Locked:              false,
	}

	var opPhone *opapi.ApplicantPhones
	if app.Phone != "" {
		opPhone = &opapi.ApplicantPhones{
			CellPhone:      app.Phone,
			PreferredPhone: "CELL",
		}
	}

	appAddress := &opapi.ApplicantAddress{Country: "USA"}

	newOPApplicant := &opapi.Applicant{
		Account: opAccount,
		Phones:  opPhone,
		Address: appAddress,
	}

	return newOPApplicant
}

func (app *IndeedJobAppPost) GetResumeFileAndExtension() (*string, string) {
	suffixArr := strings.Split(app.Applicant.Resume.File.FileName, ".")
	return &app.Applicant.Resume.File.Data, suffixArr[len(suffixArr)-1]
}

func (app *ZRJobAppPost) GetResumeFileAndExtension() (*string, string) {
	return &app.Resume, "pdf"
}

func jobApplicationPostHandler(c *gin.Context) {
	newJobApp := JobApplication{}

	sourceParam := c.Query("source")

	company := Company{}

	var jobSiteData JobSiteApplicationData

	switch sourceParam {
	case INDEED_INTEGRATION_URL:
		companyShortnameParam := c.Query("company_shortname")
		jobIDParam := c.Query("job_id")

		if companyShortnameParam == "" || jobIDParam == "" || sourceParam == "" {
			ErrorLog.Println("jobApplicationPostHandler missing param: ", companyShortnameParam, jobIDParam, sourceParam)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Missing parameters"})
			return
		}

		var err error
		company, err = lookupCompanyByShortname(companyShortnameParam)
		if err != nil {
			ErrorLog.Println("lookupCompanyByShortname err: ", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Not found"})
			return
		}

		input := &IndeedJobAppPost{}
		if err := c.ShouldBindWith(input, binding.JSON); err != nil {
			ErrorLog.Println("err binding indeed json: ", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
			return
		}

		jobSiteData = input

		existing, err := lookupJobAppBySourceID(input.ID, sourceParam)
		if err == nil {
			ErrorLog.Printf("got indeed app but its duplicate id: %+v\n", existing)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Duplicate"})
			return
		}

		newJobApp.CompanyID = company.ID
		newJobApp.OPJobID = jobIDParam
		newJobApp.Source = "Indeed"
		newJobApp.SourceApplicationID = input.ID
		newJobApp.AppliedOnMillis = input.AppliedOnMillis
		newJobApp.ApplicantName = input.Applicant.FullName

		newJobApp.ApplicantEmail = input.Applicant.Email
		newJobApp.ApplicantPhoneNumber = input.Applicant.PhoneNumber

		resumeData, _ := json.Marshal(input.Applicant.Resume.JSON)
		var resumeInterface map[string]interface{}
		json.Unmarshal(resumeData, &resumeInterface)

		// to save storage space, since we do not display this
		resumeInterface["links"] = nil
		resumeInterface["awards"] = nil
		resumeInterface["certifications"] = nil
		resumeInterface["associations"] = nil
		resumeInterface["patents"] = nil
		resumeInterface["publications"] = nil
		resumeInterface["militaryServices"] = nil

		suffixArr := strings.Split(input.Applicant.Resume.File.FileName, ".")
		if len(suffixArr) > 1 && input.Applicant.Resume.File.Data != "" {
			fileString, resumeFileExtension := input.GetResumeFileAndExtension()

			fileName, err := uploadResumeFiles(resumeFileExtension, newJobApp, fileString)
			if err != nil {
				ErrorLog.Println("uploadResumeFiles err: " + err.Error())
			} else {
				InfoLog.Println("resume uploaded: " + fileName)
				url := fmt.Sprintf("%s%s", RESUMES_URL, fileName)
				newJobApp.ResumeURL = &url
			}
		} else {
			ErrorLog.Println("indeed filename split err : " + err.Error())
			if input.Applicant.Resume.File.Data == "" {
				ErrorLog.Println("indeed resume was empty! ")
			}
		}

		newJobApp.Data = resumeInterface
	case ZIPRECRUITER_INTEGRATION_URL:
		input := &ZRJobAppPost{}
		if err := c.ShouldBindWith(&input, binding.JSON); err != nil {
			ErrorLog.Println("err binding indeed json: ", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
			return
		}

		if len(input.JobID) <= 6 {
			ErrorLog.Println("ziprecreuiter chop off last 6 chars not enough letters: ", input.JobID)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
			return
		}

		jobSiteData = input

		inputJobIDWithoutSuffix := input.JobID[:(len(input.JobID) - 6)]

		existing, err := lookupJobAppBySourceID(input.AppID, sourceParam)
		if err == nil {
			ErrorLog.Printf("got indeed app but its duplicate id: %+v\n", existing)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Duplicate"})
			return
		}

		// below is needs to get the company of this job
		thisJobInDB := CurrentJobReq{}
		err = dbmap.SelectOne(&thisJobInDB, "SELECT * FROM current_job_reqs WHERE op_id = ?", inputJobIDWithoutSuffix)
		if err != nil {
			ErrorLog.Println("ZipRecruiter sent application of job that is not in current jobs - id: ", inputJobIDWithoutSuffix)
			ErrorLog.Printf("app sent: %+v\n", input)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Job Not Found"})
			return
		}

		company, _ = lookupCompanyByOPID(thisJobInDB.CompanyOPID)

		newJobApp.CompanyID = company.ID
		newJobApp.OPJobID = inputJobIDWithoutSuffix
		newJobApp.Source = "ZipRecruiter"
		newJobApp.SourceApplicationID = input.AppID
		newJobApp.AppliedOnMillis = time.Now().Unix() * 1000
		newJobApp.ApplicantName = input.Name
		newJobApp.ApplicantFirstName = input.FirstName
		newJobApp.ApplicantLastName = input.LastName
		newJobApp.ApplicantEmail = input.Email
		newJobApp.ApplicantPhoneNumber = input.Phone

		if input.Resume == "" {
			ErrorLog.Println("ZR resume was empty!")
		} else {
			fileString, resumeFileExtension := input.GetResumeFileAndExtension()

			fileName, err := uploadResumeFiles(resumeFileExtension, newJobApp, fileString)
			if err != nil {
				ErrorLog.Println("uploadResumeFiles err: " + err.Error())
			} else {
				InfoLog.Println("resume uploaded: " + fileName)
				url := fmt.Sprintf("%s%s", RESUMES_URL, fileName)
				newJobApp.ResumeURL = &url
			}
		}

		newJobApp.Data = PropertyMap{}
	default:
		ErrorLog.Println("jobApplicationPostHandler sourceParam did not match nay integration: ", sourceParam)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not Found"})
		return
	}

	fal := false
	newJobApp.AppliedOnOP = &fal
	newJobApp.FollowUpEmailSent = &fal
	newJobApp.UserReviewed = &fal
	err := dbmap.Insert(&newJobApp)
	if err != nil {
		ErrorLog.Printf("err inserting new job app: %+v\n", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "An error occured"})
		return
	}

	InfoLog.Printf("New applicant - Company: %s, Name: %s, Source: %s\n", company.ShortName, newJobApp.ApplicantName, newJobApp.Source)

	if env.Production {
		go offlineProcessNewApplicant(company, &newJobApp)
	} else if NEW_APPLICANT_PROCESS {
		go offlineProcessNewApplicantV2(company, &newJobApp, jobSiteData)
	}

	c.JSON(200, gin.H{"msg": "Thanks"})
}

func offlineProcessNewApplicantV2(company Company, newJobApp *JobApplication, jobSiteData JobSiteApplicationData) {
	logPrefix := "offlineProcessNewApplicantV2 company=" + company.ShortName + " "

	alreadyCreatedApplicantID := newJobApp.applicantHasBeenAddedToOnePoint()
	if alreadyCreatedApplicantID != nil {
		newJobApp.OPApplicantID = alreadyCreatedApplicantID
		dbmap.Update(newJobApp)

		InfoLog.Println(logPrefix, "Applicant already added to OnePoint by our integration, op_id = ", *alreadyCreatedApplicantID)

		err := newJobApp.createOPApplication(company.OPID)
		if err != nil {
			ErrorLog.Println(logPrefix, "applicant id:", *newJobApp.OPApplicantID, " err createOPApplication: ", err)
			return
		} else {
			InfoLog.Println(logPrefix, "applicant id:", *newJobApp.OPApplicantID, " createOPApplication worked!")
		}
		return
	}

	newOPApplicant := jobSiteData.ConvertToOPApplicant()

	cxn := chooseOPAPICxn(company.OPID)

	applicantAlreadyInOP := false
	err, opErr, resp := cxn.CreateApplicant(company.OPID, newOPApplicant)
	if err != nil {
		if opErr != nil {
			for _, opErrS := range opErr.Errors {
				if opErrS.Code == 40408 {
					// TODO, will get here w err "'Username' with value 'jaredshahbazian@gmail.com' must be unique." if duplicate
					// if this occurs we should still add application
					applicantAlreadyInOP = true
					break
				}
			}

			if !applicantAlreadyInOP {
				ErrorLog.Println(logPrefix, "cxn.CreateApplicant non duplicate opErr: ", opErr)
				return
			}
		} else {
			ErrorLog.Println(logPrefix, "cxn.CreateApplicant err but opErr nil: ", err)
			return
		}
	}

	if !applicantAlreadyInOP {
		newAppplicantOPID := opapi.GetCreatedApplicantIDFromHeaders(resp.Header)
		if newAppplicantOPID == nil {
			ErrorLog.Println(logPrefix, "GetCreatedApplicantIDFromHeaders returned nil, going to quit process...")
			return
		}

		newJobApp.OPApplicantID = newAppplicantOPID
		dbmap.Update(newJobApp)

		InfoLog.Println(logPrefix, "created applicant w OPID: ", *newAppplicantOPID)

		err = newJobApp.uploadResumeToOnePoint(&company, jobSiteData)
		if err != nil {
			ErrorLog.Println(logPrefix, " err uploadResumeToOnePoint: ", err)
			return
		}

		InfoLog.Println(logPrefix, "applicant id:", *newJobApp.OPApplicantID, " resume upload worked!")
	} else {
		InfoLog.Println(logPrefix, "OP create applicant duplicate err with username: ", newOPApplicant.Account.Username)

		// already in OP but we dont know applicant account id
		// only current solution is to get list of applicants and match by username

		foundApplicant := cxn.FindApplicantByUsername(company.OPID, newOPApplicant.Account.Username)

		if foundApplicant == nil {
			InfoLog.Println(logPrefix, "duplicate applicant err applicant NOT FOUND with username: ", newOPApplicant.Account.Username, ", quiting...")
			return
		}

		id64 := int64(foundApplicant.Account.AccountID)
		newJobApp.OPApplicantID = &id64
		dbmap.Update(newJobApp)
	}

	err = newJobApp.createOPApplication(company.OPID)
	if err != nil {
		ErrorLog.Println(logPrefix, "applicant id:", *newJobApp.OPApplicantID, " err createOPApplication: ", err)
		return
	} else {
		InfoLog.Println(logPrefix, "applicant id:", *newJobApp.OPApplicantID, " createOPApplication worked!")
	}
}

// JobApplication must have OPApplicantID and OPJobID
func (app *JobApplication) createOPApplication(opCompanyID int64) error {
	opJobReqID, err := strconv.Atoi(app.OPJobID)
	if err != nil {
		return err
	}
	cxn := chooseOPAPICxn(opCompanyID)

	reqBody := &opapi.ApplicationPOSTBody{
		Account:        opapi.IDValue{ID: int(*app.OPApplicantID)},
		JobRequisition: &opapi.IDValue{ID: opJobReqID},
	}

	err, _, _ = cxn.CreateApplication(opCompanyID, reqBody)
	if err != nil {
		return err
	}

	return nil
}

type DocTypeSetting struct {
	ID          *int64 `json:"id"`
	DisplayName string `json:"display_name"`
}

func (app *JobApplication) uploadResumeToOnePoint(company *Company, jobSiteData JobSiteApplicationData) error {
	fileString, resumeFileExtension := jobSiteData.GetResumeFileAndExtension()

	if fileString == nil {
		return errors.New("file nil")
	}

	coProd, err := getCompanyProductByURL(app.CompanyID, "jobs")
	if err != nil {
		return err
	}

	docTypeSettingM, ok := coProd.Settings["company_resume_doctype_id"].(map[string]interface{})
	if !ok {
		return errors.New("Could not typecast company_resume_doctype_id")
	}

	docTypeSetting := mapToDocTypeSetting(docTypeSettingM)

	cxn := chooseOPAPICxn(company.OPID)

	if docTypeSetting.ID == nil {
		resumeDocType := cxn.FindDocTypeByName(company.OPID, "resume")
		if resumeDocType == nil {
			return errors.New("company did not have doc type named resume")
		}
		newSetting := &DocTypeSetting{
			ID:          &resumeDocType.ID,
			DisplayName: resumeDocType.DisplayName,
		}
		docTypeSetting = newSetting
		coProd.Settings["company_resume_doctype_id"] = newSetting
		dbmap.Update(&coProd)
	}

	newDocRequest := opapi.NewDocumentRequest{
		Type:        "HR_APPLICANT_RESUME",
		FileName:    fmt.Sprintf("resume.%s", resumeFileExtension),
		DisplayName: fmt.Sprintf("%s Resume (from %s)", app.ApplicantName, app.Source),
		DocumentType: opapi.DocType{
			ID:          *docTypeSetting.ID,
			DisplayName: docTypeSetting.DisplayName,
		},
		LinkedID: strconv.FormatInt(*app.OPApplicantID, 10),
	}

	decoded, err := base64.StdEncoding.DecodeString(*fileString)
	if err != nil {
		return err
	}
	err, opErr := cxn.UploadDocument(company.OPID, &newDocRequest, decoded)
	if err != nil {
		if opErr != nil {
			return errors.New(opErr.String())
		}
		return err
	}

	return nil
}

func mapToDocTypeSetting(pm map[string]interface{}) *DocTypeSetting {
	var dts DocTypeSetting
	datab, err := json.Marshal(pm)
	if err != nil {
		return nil
	}
	err = json.Unmarshal(datab, &dts)
	if err != nil {
		return nil
	}

	return &dts
}

// Returns the OnePoint Applicant ID if we have already created this applicant, nil if not
func (app *JobApplication) applicantHasBeenAddedToOnePoint() *int64 {
	apps := []JobApplication{}
	dbmap.Select(&apps, "SELECT id, company_id, op_applicant_id FROM job_applications WHERE applicant_email = ? AND company_id = ? AND !ISNULL(op_applicant_id)", app.ApplicantEmail, app.CompanyID)
	if len(apps) > 0 {
		return apps[0].OPApplicantID
	}
	return nil
}

func offlineProcessNewApplicant(company Company, newJobApp *JobApplication) {
	if newJobApp.ApplicantEmail == "cehovin.matteo@gmail.com" || strings.HasSuffix(newJobApp.ApplicantEmail, "indeed.com") || strings.HasSuffix(newJobApp.ApplicantEmail, "ziprecruiter.com") {
		InfoLog.Println("offlineProcessNewApplicant, EMAIL WARNING !! MATTEO OR INDEED: " + newJobApp.ApplicantEmail)
		return
	}

	err := sendFinishEmailAndUpdateSent(newJobApp, company)
	if err != nil {
		ErrorLog.Printf("offlineProcessNewApplicant err: %+v\n", err)
	} else {
		InfoLog.Println("sendFinishEmailAndUpdateSent successful")
	}
}

func uploadResumeFiles(fileExtension string, jobApp JobApplication, resumeBase64 *string) (string, error) {
	if !env.Production {
		return "", nil
	}

	decoded, err := base64.StdEncoding.DecodeString(*resumeBase64)
	if err != nil {
		return "", err
	}

	// determine name
	fileName := fmt.Sprintf("res%d%s%s.%s", jobApp.AppliedOnMillis, jobApp.OPJobID, jobApp.SourceApplicationID, fileExtension)

	err = bytesToGCP(JOB_RESUMES_BUCKETNAME, fileName, decoded, true)
	if err != nil {
		return "", err
	}

	return fileName, nil
}

func sendEmployerJobAppNotficationEmail(email, applicantName, jobTitle, sourceName string) {
	emailHeaderInfo := sgEmailFields{
		From:    &mail.Email{Name: "OnePoint Connect", Address: passwords.NO_REPLY_EMAILER_ADDRESS},
		To:      []*mail.Email{&mail.Email{Address: email}},
		Subject: "New " + jobTitle + " Applicant: " + applicantName,
	}

	body := ApplicationNotificationEmailBody{
		ApplicantName: applicantName,
		JobTitle:      jobTitle,
		Source:        sourceName,
	}

	err := sendTemplatedEmailSendGrid(emailHeaderInfo, JOB_APPLICATION_EMAIL_TEMPLATE, body)
	if err != nil {
		ErrorLog.Println("sendJobAppNotficationEmail email err: ", err)
	}
}

func lookupJobAppBySourceID(sourceID, source string) (JobApplication, error) {
	existingJob := JobApplication{}
	err := dbmap.SelectOne(&existingJob, "SELECT * FROM job_applications WHERE source = ? AND source_application_id = ?", source, sourceID)
	return existingJob, err
}

func findAllJobsAppsByOPID(opJobIP string, page int) (int64, []JobApplication, error) {
	count, err := dbmap.SelectInt("SELECT COUNT(*) FROM job_applications WHERE op_job_id = ?", opJobIP)
	jobApps := []JobApplication{}

	if err != nil {
		ErrorLog.Println("err in find count of jobs apps: " + err.Error())
		return count, jobApps, err
	}

	countToGet := page * JOB_APPS_COUNT

	_, err = dbmap.Select(&jobApps, "SELECT * FROM job_applications WHERE op_job_id = ? ORDER BY applied_on_millis DESC LIMIT 0, ?", opJobIP, countToGet)
	if err != nil {
		ErrorLog.Printf("findAllJobsAppsByOPID job id: %s, countToGet: %d, SQL err: %+v\n", opJobIP, countToGet, err)
	}

	return count, jobApps, err
}
