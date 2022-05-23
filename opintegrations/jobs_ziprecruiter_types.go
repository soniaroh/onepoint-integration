package main

import (
	"encoding/xml"
)

type ZRJobAppPost struct {
	AppID     string `json:"response_id"`
	JobID     string `json:"job_id"`
	Name      string `json:"name"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	Resume    string `json:"resume"`
}

type ZRXMLSource struct {
	XMLName       xml.Name `xml:"source"`
	Publisher     string   `xml:"publisher"`
	PublisherURL  string   `xml:"publisherurl"`
	LastBuildDate string   `xml:"lastbuilddate"`
	Jobs          []ZRXMLJob
}

type ZRXMLJob struct {
	XMLName                   xml.Name      `xml:"job"`
	ReferenceNumber           string        `xml:"referencenumber"`
	Title                     string        `xml:"title"`
	Description               ZRDescription `xml:"description"`
	Country                   string        `xml:"country"`
	City                      string        `xml:"city"`
	State                     string        `xml:"state"`
	PostalCode                string        `xml:"postalcode"`
	Company                   string        `xml:"company"`
	Date                      string        `xml:"date"`
	Category                  ZRCategory    `xml:"category"`
	URL                       ZRURL         `xml:"url"`
	JobType                   string        `xml:"jobtype"`
	Experience                string        `xml:"experience"`
	Education                 string        `xml:"education"`
	CompensationInterval      string        `xml:"compensation_interval"`
	CompensationMin           string        `xml:"compensation_min"`
	CompensationMax           string        `xml:"compensation_max"`
	CompensationHasCommission string        `xml:"compensation_has_commission"`
	Sponsored                 *string       `xml:"trafficboost"`
}

type ZRTitle struct {
	XMLName xml.Name `xml:"title"`
	Text    string   `xml:",cdata"`
}

type ZRDate struct {
	XMLName xml.Name `xml:"date"`
	Text    string   `xml:",cdata"`
}

type ZRReferenceNumber struct {
	XMLName xml.Name `xml:"referencenumber"`
	Text    string   `xml:",cdata"`
}

type ZRURL struct {
	XMLName xml.Name `xml:"url"`
	Text    string   `xml:",cdata"`
}

type ZRCompany struct {
	XMLName xml.Name `xml:"company"`
	Text    string   `xml:",cdata"`
}

type ZRCity struct {
	XMLName xml.Name `xml:"city"`
	Text    string   `xml:",cdata"`
}

type ZRState struct {
	XMLName xml.Name `xml:"state"`
	Text    string   `xml:",cdata"`
}

type ZRCountry struct {
	XMLName xml.Name `xml:"country"`
	Text    string   `xml:",cdata"`
}

type ZRPostalCode struct {
	XMLName xml.Name `xml:"postalcode"`
	Text    string   `xml:",cdata"`
}

type ZREmail struct {
	XMLName xml.Name `xml:"email"`
	Text    string   `xml:",cdata"`
}

type ZRDescription struct {
	XMLName xml.Name `xml:"description"`
	Text    string   `xml:",cdata"`
}

type ZRSalary struct {
	XMLName xml.Name `xml:"salary"`
	Text    string   `xml:",cdata"`
}

type ZREducation struct {
	XMLName xml.Name `xml:"education"`
	Text    string   `xml:",cdata"`
}

type ZRCategory struct {
	XMLName xml.Name `xml:"category"`
	Text    string   `xml:",cdata"`
}

type ZRExperience struct {
	XMLName xml.Name `xml:"experience"`
	Text    string   `xml:",cdata"`
}

type ZRSponsored struct {
	XMLName xml.Name `xml:"sponsored"`
	Text    string   `xml:",cdata"`
}

type ZRBudget struct {
	XMLName xml.Name `xml:"budget"`
	Text    string   `xml:",cdata"`
}

type ZRPhone struct {
	XMLName xml.Name `xml:"phone"`
	Text    string   `xml:",cdata"`
}
