package main

import (
	"encoding/xml"
)

type IndeedXMLSource struct {
	XMLName       xml.Name `xml:"source"`
	Publisher     string   `xml:"publisher"`
	PublisherURL  string   `xml:"publisherurl"`
	LastBuildDate string   `xml:"lastbuilddate"`
	Jobs          []IndeedXMLJob
}

type IndeedXMLJob struct {
	XMLName         xml.Name              `xml:"job"`
	Title           IndeedTitle           `xml:"title"`
	Date            IndeedDate            `xml:"date"`
	ReferenceNumber IndeedReferenceNumber `xml:"referencenumber"`
	URL             IndeedURL             `xml:"url"`
	Company         IndeedCompany         `xml:"company"`
	SourceName      IndeedSourceName      `xml:"sourcename"`
	City            IndeedCity            `xml:"city"`
	State           IndeedState           `xml:"state"`
	Country         IndeedCountry         `xml:"country"`
	PostalCode      IndeedPostalCode      `xml:"postalcode"`
	Email           IndeedEmail           `xml:"email"`
	Description     IndeedDescription     `xml:"description"`
	Salary          IndeedSalary          `xml:"salary"`
	Education       IndeedEducation       `xml:"education"`
	Category        IndeedCategory        `xml:"category"`
	Experience      IndeedExperience      `xml:"experience"`
	IndeedApplyData IndeedApplyData       `xml:"indeed-apply-data"`
	Sponsored       IndeedSponsored       `xml:"sponsored"`
	Budget          IndeedBudget          `xml:"budget"`
	ContactName     IndeedContactName     `xml:"contact"`
	Phone           IndeedPhone           `xml:"phone"`
}

type IndeedJobAppPost struct {
	ID              string `json:"id"`
	Locale          string `json:"locale"`
	AppliedOnMillis int64  `json:"appliedOnMillis"`
	Job             struct {
		JobID       string `json:"jobId"`
		JobTitle    string `json:"jobTitle"`
		JobCompany  string `json:"jobCompany"`
		JobLocation string `json:"jobLocation"`
		JobURL      string `json:"jobUrl"`
		JobMeta     string `json:"jobMeta"`
	} `json:"job"`
	Applicant struct {
		FullName    string `json:"fullName"`
		Email       string `json:"email"`
		PhoneNumber string `json:"phoneNumber"`
		Coverletter string `json:"coverletter"`
		Resume      struct {
			Text  string `json:"text"`
			HrXML string `json:"hrXml"`
			HTML  string `json:"html"`
			JSON  struct {
				FirstName        string `json:"firstName"`
				LastName         string `json:"lastName"`
				Headline         string `json:"headline"`
				Summary          string `json:"summary"`
				PublicProfileURL string `json:"publicProfileUrl"`
				AdditionalInfo   string `json:"additionalInfo"`
				PhoneNumber      string `json:"phoneNumber"`
				Location         struct {
					City string `json:"city"`
				} `json:"location"`
				Skills    string `json:"skills"`
				Positions struct {
					Total  int `json:"_total"`
					Values []struct {
						Title          string `json:"title"`
						Company        string `json:"company"`
						Location       string `json:"location"`
						StartDateMonth string `json:"startDateMonth"`
						StartDateYear  string `json:"startDateYear"`
						EndDateMonth   string `json:"endDateMonth"`
						EndDateYear    string `json:"endDateYear"`
						EndCurrent     bool   `json:"endCurrent"`
						Description    string `json:"description"`
					} `json:"values"`
				} `json:"positions"`
				Educations struct {
					Total  int `json:"_total"`
					Values []struct {
						Degree     string `json:"degree"`
						Field      string `json:"field"`
						School     string `json:"school"`
						Location   string `json:"location"`
						StartDate  string `json:"startDate"`
						EndDate    string `json:"endDate"`
						EndCurrent bool   `json:"endCurrent"`
					} `json:"values"`
				} `json:"educations"`
				Links struct {
					Total  int `json:"_total"`
					Values []struct {
						URL string `json:"url"`
					} `json:"values"`
				} `json:"links"`
				Awards struct {
					Total  int `json:"_total"`
					Values []struct {
						Title       string `json:"title"`
						DateMonth   string `json:"dateMonth"`
						DateYear    string `json:"dateYear"`
						Description string `json:"description"`
					} `json:"values"`
				} `json:"awards"`
				Certifications struct {
					Total  int `json:"_total"`
					Values []struct {
						Title          string `json:"title"`
						StartDateMonth string `json:"startDateMonth"`
						StartDateYear  string `json:"startDateYear"`
						EndDateMonth   string `json:"endDateMonth"`
						EndDateYear    string `json:"endDateYear"`
						EndCurrent     bool   `json:"endCurrent"`
						Description    string `json:"description"`
					} `json:"values"`
				} `json:"certifications"`
				Associations struct {
					Total  int `json:"_total"`
					Values []struct {
						Title          string `json:"title"`
						StartDateMonth string `json:"startDateMonth"`
						StartDateYear  string `json:"startDateYear"`
						EndDateMonth   string `json:"endDateMonth"`
						EndDateYear    string `json:"endDateYear"`
						EndCurrent     bool   `json:"endCurrent"`
						Description    string `json:"description"`
					} `json:"values"`
				} `json:"associations"`
				Patents struct {
					Total  int `json:"_total"`
					Values []struct {
						PatentNumber string `json:"patentNumber"`
						Title        string `json:"title"`
						URL          string `json:"url"`
						DateMonth    string `json:"dateMonth"`
						DateYear     string `json:"dateYear"`
						Description  string `json:"description"`
					} `json:"values"`
				} `json:"patents"`
				Publications struct {
					Total  int `json:"_total"`
					Values []struct {
						Title       string `json:"title"`
						URL         string `json:"url"`
						DateDay     string `json:"dateDay"`
						DateMonth   string `json:"dateMonth"`
						DateYear    string `json:"dateYear"`
						Description string `json:"description"`
					} `json:"values"`
				} `json:"publications"`
				MilitaryServices struct {
					Total  int `json:"_total"`
					Values []struct {
						ServiceCountry string `json:"serviceCountry"`
						Branch         string `json:"branch"`
						Rank           string `json:"rank"`
						StartDateMonth string `json:"startDateMonth"`
						StartDateYear  string `json:"startDateYear"`
						EndDateMonth   string `json:"endDateMonth"`
						EndDateYear    string `json:"endDateYear"`
						EndCurrent     bool   `json:"endCurrent"`
						Commendations  string `json:"commendations"`
						Description    string `json:"description"`
					} `json:"values"`
				} `json:"militaryServices"`
			} `json:"json"`
			File struct {
				ContentType string `json:"contentType"`
				Data        string `json:"data"`
				FileName    string `json:"fileName"`
			} `json:"file"`
		} `json:"resume"`
	} `json:"applicant"`
}

type IndeedTitle struct {
	XMLName xml.Name `xml:"title"`
	Text    string   `xml:",cdata"`
}

type IndeedDate struct {
	XMLName xml.Name `xml:"date"`
	Text    string   `xml:",cdata"`
}

type IndeedReferenceNumber struct {
	XMLName xml.Name `xml:"referencenumber"`
	Text    string   `xml:",cdata"`
}

type IndeedURL struct {
	XMLName xml.Name `xml:"url"`
	Text    string   `xml:",cdata"`
}

type IndeedCompany struct {
	XMLName xml.Name `xml:"company"`
	Text    string   `xml:",cdata"`
}

type IndeedSourceName struct {
	XMLName xml.Name `xml:"sourcename"`
	Text    string   `xml:",cdata"`
}

type IndeedCity struct {
	XMLName xml.Name `xml:"city"`
	Text    string   `xml:",cdata"`
}

type IndeedState struct {
	XMLName xml.Name `xml:"state"`
	Text    string   `xml:",cdata"`
}

type IndeedCountry struct {
	XMLName xml.Name `xml:"country"`
	Text    string   `xml:",cdata"`
}

type IndeedPostalCode struct {
	XMLName xml.Name `xml:"postalcode"`
	Text    string   `xml:",cdata"`
}

type IndeedEmail struct {
	XMLName xml.Name `xml:"email"`
	Text    string   `xml:",cdata"`
}

type IndeedDescription struct {
	XMLName xml.Name `xml:"description"`
	Text    string   `xml:",cdata"`
}

type IndeedSalary struct {
	XMLName xml.Name `xml:"salary"`
	Text    string   `xml:",cdata"`
}

type IndeedEducation struct {
	XMLName xml.Name `xml:"education"`
	Text    string   `xml:",cdata"`
}

type IndeedCategory struct {
	XMLName xml.Name `xml:"category"`
	Text    string   `xml:",cdata"`
}

type IndeedExperience struct {
	XMLName xml.Name `xml:"experience"`
	Text    string   `xml:",cdata"`
}

type IndeedApplyData struct {
	XMLName xml.Name `xml:"indeed-apply-data"`
	Text    string   `xml:",cdata"`
}

type IndeedSponsored struct {
	XMLName xml.Name `xml:"sponsored"`
	Text    string   `xml:",cdata"`
}

type IndeedBudget struct {
	XMLName xml.Name `xml:"budget"`
	Text    string   `xml:",cdata"`
}

type IndeedPhone struct {
	XMLName xml.Name `xml:"phone"`
	Text    string   `xml:",cdata"`
}

type IndeedContactName struct {
	XMLName xml.Name `xml:"contact"`
	Text    string   `xml:",cdata"`
}
