package main

import (
	"bytes"
	"errors"
	"html/template"
	"path/filepath"

	sendgrid "github.com/sendgrid/sendgrid-go"
	sgmail "github.com/sendgrid/sendgrid-go/helpers/mail"
)

var templates *template.Template

type emailFields struct {
	From    string
	To      []string
	Cc      []string
	Bcc     []string
	Subject string
}

type sgEmailFields struct {
	From    *sgmail.Email
	To      []*sgmail.Email
	Cc      []*sgmail.Email
	Bcc     []*sgmail.Email
	Subject string
}

const (
	HF_SUMMARY_EMAIL_TEMPLATE              = "user_provisioning_summary.html"
	TEST_EMAIL_TEMPLATE                    = "test_template.html"
	JOB_APPLICATION_EMAIL_TEMPLATE         = "application_notification.html"
	JOB_FINISH_APP_EMAIL_TEMPLATE          = "finish_job_application.html"
	JOB_FINISH_APP_REMINDER_EMAIL_TEMPLATE = "reminder_job_application.html"
	JOB_SPONSORSHIP_NOTIFICATION_TEMPLATE  = "new_job_sponsorship.html"
	BGC_NOTIFICATION_TEMPLATE              = "new_bgc_notification.html"
	SERVICE_DISCONNECTION_ALERT_TEMPLATE   = "service_disconnected_alert.html"
	AD_HF_SUMMARY_TEMPLATE                 = "active_directory_summary.html"
	GH_ADMIN_ALERT_TEMPLATE                = "google_hire_admin_alert.html"
	GH_APPLICANT_EMAIL_TEMPLATE            = "google_hire_applicant_email.html"
)

func initEmailTemplates() {
	absPath := "/etc/opintegrations/templates/*"
	if !env.Production {
		absPath, _ = filepath.Abs("./opintegrations/templates/*")
	}

	templates = template.Must(template.ParseGlob(absPath))
}

type HiredSummaryEmailEvent struct {
	ServiceName string
	Username    string
	Password    string
}

type FiredSummaryEmailEvent struct {
	ServiceName string
	Description string
}

type HFSummaryEmailBody struct {
	HiredTrueFiredFalse bool
	EmployeeName        string
	HiredResults        []HiredSummaryEmailEvent
	FiredResults        []FiredSummaryEmailEvent
	ErrdServices        []struct {
		Name        string
		Description string
	}
}

type ApplicationNotificationEmailBody struct {
	ApplicantName string
	JobTitle      string
	Source        string
}

type FinishApplicationEmailBody struct {
	ApplicantName string
	CompanyName   string
	JobTitle      string
	ApplyURL      string
}

type NewSponsoredJobNotificationBody struct {
	CompanyName      string
	CompanyShortName string
	Price            string
}

type NewBGCNotificationBody struct {
	ApplicantName string
}

type GHAdminAlertBody struct {
	ApplicantName string
}

type GHApplicantAlertBody struct {
	Username  string
	Password  string
	LoginLink string
}

func sendTemplatedEmailSendGrid(emailInfo sgEmailFields, templateToUse string, templateData interface{}, categories ...string) error {
	temp := templates.Lookup(templateToUse)
	var tpl bytes.Buffer
	if err := temp.Execute(&tpl, templateData); err != nil {
		return errors.New("template execute err: " + err.Error())
	}
	htmlContent := tpl.String()

	m := sgmail.NewV3Mail()

	m.SetFrom(emailInfo.From)

	content := sgmail.NewContent("text/html", htmlContent)
	m.AddContent(content)

	personalization := sgmail.NewPersonalization()
	personalization.AddTos(emailInfo.To...)
	personalization.AddCCs(emailInfo.Cc...)
	personalization.AddBCCs(emailInfo.Bcc...)
	personalization.Subject = emailInfo.Subject

	m.AddPersonalizations(personalization)

	m.AddCategories(categories...)

	request := sendgrid.GetRequest(passwords.SG_EMAILER_PASSWORD, "/v3/mail/send", "https://api.sendgrid.com")
	request.Method = "POST"
	request.Body = sgmail.GetRequestBody(m)
	_, err := sendgrid.API(request)
	if err != nil {
		return errors.New("err SENDGRID API request: " + err.Error())
	}

	return nil
}
