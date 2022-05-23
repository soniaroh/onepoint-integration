package main

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

type IndeedSponsorship struct {
	ID            int64  `db:"id, primarykey, autoincrement" json:"id"`
	OPJobID       string `db:"op_job_id" json:"op_job_id"`
	CompanyID     int64  `db:"company_id" json:"company_id"`
	Budget        string `db:"budget" json:"budget"`
	Email         string `db:"email" json:"email"`
	Phone         string `db:"phone" json:"phone"`
	TimeEnd       int64  `db:"time_end" json:"time_end"`
	TimeSubmitted int64  `db:"time_submitted" json:"time_submitted"`
	ManuallyEnded *bool  `db:"manually_ended" json:"manually_ended"`
	Active        *bool  `db:"-" json:"active"`
	UserID        *int64 `db:"user_id" json:"user_id"`
	ContactName   string `db:"contact_name" json:"contact_name"`
}

type ZipRecruiterSponsorship struct {
	ID            int64  `db:"id, primarykey, autoincrement" json:"id"`
	OPJobID       string `db:"op_job_id" json:"op_job_id"`
	CompanyID     int64  `db:"company_id" json:"company_id"`
	TimeEnd       int64  `db:"time_end" json:"time_end"`
	TimeSubmitted int64  `db:"time_submitted" json:"time_submitted"`
	ManuallyEnded *bool  `db:"manually_ended" json:"manually_ended"`
	Level         string `db:"level" json:"level"`
	Active        *bool  `db:"-" json:"active"`
	UserID        *int64 `db:"user_id" json:"user_id"`
}

type IndeedSponsorshipObject struct {
	Active       *bool               `json:"active"`
	Sponsorships []IndeedSponsorship `json:"sponsorships"`
}

type ZRSponsorshipObject struct {
	Active       *bool                     `json:"active"`
	Sponsorships []ZipRecruiterSponsorship `json:"sponsorships"`
}

type IndeedSponsorshipPost struct {
	Budget       string `json:"budget"`
	TimeInterval *int   `json:"time_interval,omitempty"`
	Email        string `json:"email"`
	Phone        string `json:"phone"`
	ContactName  string `json:"contact_name"`
}

type ZipRecruiterSponsorshipPost struct {
	Level string `json:"level"`
}

type IndeedUpdateSponsorshipPost struct {
	Budget        *string `json:"budget"`
	ManuallyEnded *bool   `json:"manually_ended"`
}

func zipSponsorshipPostHandler(c *gin.Context) {
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

	input := ZipRecruiterSponsorshipPost{}
	if err := c.ShouldBindWith(&input, binding.JSON); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	if input.Level == "" || !(input.Level == "single" || input.Level == "double") {
		ErrorLog.Printf("ziprecruiter sponsorship post data level wrong: %s\n", input.Level)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	var submittingUserID *int64
	user, _ := lookupThisUser(c)
	if user != nil {
		submittingUserID = &user.ID
	}

	exactlyThirtyDaysFromNow := time.Now().AddDate(0, 0, 30)
	falseBool := false

	newSponsorship := ZipRecruiterSponsorship{
		OPJobID:       opJobID,
		TimeEnd:       exactlyThirtyDaysFromNow.Unix(),
		CompanyID:     thisCompany.ID,
		Level:         input.Level,
		ManuallyEnded: &falseBool,
		TimeSubmitted: time.Now().Unix(),
		UserID:        submittingUserID,
	}

	err = dbmap.Insert(&newSponsorship)
	if err != nil {
		ErrorLog.Println("err inserting new ZIPRECRUITER Sponsorship: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown error"})
		return
	}

	jobRec, err := getJobToReturn(thisCompany, opJobID, 1)
	if err != nil {
		ErrorLog.Println("err inserting new ZIPRECRUITER Sponsorship getJobToReturn: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown error"})
		return
	}

	go sendNewSponsorshipEmail(thisCompany, newSponsorship)

	c.JSON(200, jobRec)
	return
}

func sendNewSponsorshipEmail(company Company, newSponsorship ZipRecruiterSponsorship) {
	emailHeaderInfo := sgEmailFields{
		Subject: fmt.Sprintf("New ZipRecruiter Sponsorship: %s", company.Name),
		From:    &mail.Email{Name: "OnePoint Connect", Address: passwords.NO_REPLY_EMAILER_ADDRESS},
		To:      []*mail.Email{&mail.Email{Address: passwords.ADMIN_NOTIFICATION_EMAIL_ADDRESS}},
	}

	price := "199.00"
	if newSponsorship.Level == "double" {
		price = "299.00"
	}

	emailBody := NewSponsoredJobNotificationBody{
		CompanyName:      company.Name,
		CompanyShortName: company.ShortName,
		Price:            price,
	}

	err := sendTemplatedEmailSendGrid(emailHeaderInfo, JOB_SPONSORSHIP_NOTIFICATION_TEMPLATE, emailBody)
	if err != nil {
		ErrorLog.Printf("sendNewSponsorshipEmail emailing err: %v\n", err)
	}
}

func indeedSponsorshipPostHandler(c *gin.Context) {
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

	input := IndeedSponsorshipPost{}
	if err := c.ShouldBindWith(&input, binding.JSON); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	if input.Budget == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Budget is required"})
		return
	}

	budgetInt, err := strconv.Atoi(input.Budget)
	if err != nil {
		ErrorLog.Println("indeedSponsorshipPostHandler budget was not int, err: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Budget is required"})
		return
	}

	if budgetInt < 200 {
		ErrorLog.Println("indeedSponsorshipPostHandler budget was below 200: ", budgetInt)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Budget must be greater than $200"})
		return
	}

	exactlySixteenDaysFromNow := time.Now().AddDate(0, 0, 16)
	loc, _ := time.LoadLocation("UTC")
	timeToEnd := time.Date(exactlySixteenDaysFromNow.Year(), exactlySixteenDaysFromNow.Month(), exactlySixteenDaysFromNow.Day(), 0, 0, 0, 0, loc)

	if input.TimeInterval != nil {
		// if not sent, go with 15 since that was default v1
		exactDaysFromNow := time.Now().AddDate(0, 0, *input.TimeInterval+1)
		timeToEnd = time.Date(exactDaysFromNow.Year(), exactDaysFromNow.Month(), exactDaysFromNow.Day(), 0, 0, 0, 0, loc)
	}

	var submittingUserID *int64
	contactName := ""
	user, _ := lookupThisUser(c)
	if user != nil {
		submittingUserID = &user.ID
		contactName = fmt.Sprintf("%s %s", user.FirstName, user.LastName)
	} else {
		contactName = input.ContactName
	}

	falseBool := false

	newSponsorship := IndeedSponsorship{
		OPJobID:       opJobID,
		Budget:        input.Budget,
		Email:         input.Email,
		Phone:         input.Phone,
		TimeEnd:       timeToEnd.Unix(),
		TimeSubmitted: time.Now().Unix(),
		ManuallyEnded: &falseBool,
		CompanyID:     thisCompany.ID,
		UserID:        submittingUserID,
		ContactName:   contactName,
	}

	err = dbmap.Insert(&newSponsorship)
	if err != nil {
		ErrorLog.Println("err inserting new iNDEED Sponsorship: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown error"})
		return
	}

	jobRec, err := getJobToReturn(thisCompany, opJobID, 1)
	if err != nil {
		ErrorLog.Println("err inserting new iNDEED Sponsorship getJobToReturn: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown error"})
		return
	}

	c.JSON(200, jobRec)
	return
}

func indeedSponsorshipUpdateHandler(c *gin.Context) {
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

	sponsorshipID := c.Param("sponsorshipID")
	if sponsorshipID == "" {
		ErrorLog.Println("blank sponsorshipID path parameter")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	input := IndeedUpdateSponsorshipPost{}
	if err := c.ShouldBindWith(&input, binding.JSON); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	sponsorshipIDint, err := strconv.Atoi(sponsorshipID)
	if err != nil {
		ErrorLog.Println(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	thisSponsorship, err := lookupSponsorshipByID(int64(sponsorshipIDint))
	if err != nil {
		ErrorLog.Println(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	if input.Budget != nil {
		thisSponsorship.Budget = *input.Budget
	}

	if input.ManuallyEnded != nil {
		thisSponsorship.ManuallyEnded = input.ManuallyEnded
	}

	_, err = dbmap.Update(&thisSponsorship)
	if err != nil {
		ErrorLog.Println(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "An error occured"})
		return
	}

	jobRec, err := getJobToReturn(thisCompany, opJobID, 1)
	if err != nil {
		ErrorLog.Println("err inserting new iNDEED Sponsorship getJobToReturn: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown error"})
		return
	}

	c.JSON(200, jobRec)
	return
}

func findAllIndeedSponsorshipByOPID(opJobIP string) (IndeedSponsorshipObject, error) {
	IndeedSponsorship := []IndeedSponsorship{}
	_, err := dbmap.Select(&IndeedSponsorship, "SELECT * FROM indeed_sponsorships WHERE op_job_id = ? ORDER BY time_end DESC", opJobIP)

	isActive := false

	falseBool := false
	trueBool := true
	nowSecs := time.Now().Unix()

	for index := 0; index < len(IndeedSponsorship); index++ {
		if nowSecs < IndeedSponsorship[index].TimeEnd && !*IndeedSponsorship[index].ManuallyEnded {
			IndeedSponsorship[index].Active = &trueBool
			isActive = true
		} else {
			IndeedSponsorship[index].Active = &falseBool
		}
	}

	indeedObj := IndeedSponsorshipObject{
		Active:       &isActive,
		Sponsorships: IndeedSponsorship,
	}

	return indeedObj, err
}

func findAllZRSponsorshipByOPID(opJobIP string) (ZRSponsorshipObject, error) {
	ZipRecruiterSponsorships := []ZipRecruiterSponsorship{}
	_, err := dbmap.Select(&ZipRecruiterSponsorships, "SELECT * FROM ziprecruiter_sponsorships WHERE op_job_id = ? ORDER BY time_end DESC", opJobIP)

	isActive := false

	falseBool := false
	trueBool := true
	nowSecs := time.Now().Unix()

	for index := 0; index < len(ZipRecruiterSponsorships); index++ {
		if nowSecs < ZipRecruiterSponsorships[index].TimeEnd && !*ZipRecruiterSponsorships[index].ManuallyEnded {
			ZipRecruiterSponsorships[index].Active = &trueBool
			isActive = true
		} else {
			ZipRecruiterSponsorships[index].Active = &falseBool
		}
	}

	zrObj := ZRSponsorshipObject{
		Active:       &isActive,
		Sponsorships: ZipRecruiterSponsorships,
	}

	return zrObj, err
}

func lookupSponsorshipByID(sponsorshipID int64) (IndeedSponsorship, error) {
	existingS := IndeedSponsorship{}
	err := dbmap.SelectOne(&existingS, "SELECT * FROM indeed_sponsorships WHERE id = ?", sponsorshipID)
	return existingS, err
}
