package main

import "time"

type GHApplicationListResult struct {
	Applications []GHApplication `json:"applications"`
}

type GHApplication struct {
	Name         string    `json:"name"`
	Job          string    `json:"job"`
	Candidate    string    `json:"candidate"`
	CreateTime   time.Time `json:"createTime"`
	CreatingUser string    `json:"creatingUser"`
	Status       GHStatus  `json:"status"`
}
type GHProcessStage struct {
	Stage             string `json:"stage"`
	ReportingCategory string `json:"reportingCategory"`
}
type GHStatus struct {
	State        string         `json:"state"`
	ProcessStage GHProcessStage `json:"processStage"`
	UpdateTime   time.Time      `json:"updateTime"`
}

type GHCandidate struct {
	Name       string    `json:"name"`
	CreateTime time.Time `json:"createTime"`
	PersonName struct {
		GivenName  string `json:"givenName"`
		FamilyName string `json:"familyName"`
	} `json:"personName"`
	Email                  string   `json:"email"`
	AdditionalEmails       []string `json:"additionalEmails"`
	PhoneNumber            string   `json:"phoneNumber"`
	AdditionalPhoneNumbers []string `json:"additionalPhoneNumbers"`
	PostalAddresses        []struct {
		PostalCode         string   `json:"postalCode"`
		AdministrativeArea string   `json:"administrativeArea"`
		Locality           string   `json:"locality"`
		AddressLines       []string `json:"addressLines,omitempty"`
	} `json:"postalAddresses"`
	EmploymentInfo []struct {
		JobTitle  string `json:"jobTitle"`
		Employer  string `json:"employer,omitempty"`
		StartDate struct {
			Year  int `json:"year"`
			Month int `json:"month"`
			Day   int `json:"day"`
		} `json:"startDate"`
		EndDate struct {
			Year  int `json:"year"`
			Month int `json:"month"`
			Day   int `json:"day"`
		} `json:"endDate"`
	} `json:"employmentInfo"`
	EducationInfo []struct {
		School string `json:"school"`
	} `json:"educationInfo"`
	Source struct {
		Type  string `json:"type"`
		Label string `json:"label"`
	} `json:"source"`
	Applications []string `json:"applications"`
}

type GHNewRegRequest struct {
	Registration GHRegistration `json:"registration"`
}
type GHRegistration struct {
	NotificationTypes []string `json:"notificationTypes"`
	PubsubTopic       string   `json:"pubsubTopic"`

	Name       *string `json:"name,omitempty"`       // output only
	ExpireTime *string `json:"expireTime,omitempty"` // output only
}

type GoogleHireWebhookBody struct {
	Message      GoogleHireWebhookMessage `json:"message"`
	Subscription string                   `json:"subscription"`
}
type GoogleHireWebhookMessage struct {
	Data         string    `json:"data"`
	MessageID    string    `json:"messageId"`
	MessageIDu   string    `json:"message_id"` // they send two formats for some reason
	PublishTime  time.Time `json:"publishTime"`
	PublishTimeu time.Time `json:"publish_time"`
}
type GHWebhookMessageData struct {
	NotificationType string `json:"notificationType"`
	Registration     string `json:"registration"`
	Resource         string `json:"resource"`
}
