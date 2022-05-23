package main

import (
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	INDEED_API_TOKEN             = "09274bbfc7d87deadde7a91d882cbb337762d8cec8d48702e130c1ce4f70e0ec"
	INDEED_FEED_BUCKET_NAME      = "onepoint-jobs"
	INDEED_INTEGRATION_URL       = "indeed"
	ZIPRECRUITER_INTEGRATION_URL = "ziprecruiter"
	JOBS_PRODUCT_URL             = "jobs"
)

// see http://opensource.indeedeng.io/api-documentation/docs/xml-feed-ia/

func checkCompanyBlockedJobsFeeds(companyID int64) bool {
	qry := `SELECT * FROM companies WHERE short_name IN ("ZJARED", "ZTRNING", "ZKTest")`
	blockedCompanies := []Company{}
	_, err := dbmap.Select(&blockedCompanies, qry)
	if err != nil {
		ErrorLog.Println("ERR GETTING BLOCKED COMPANIES!: " + err.Error())
	}

	for _, co := range blockedCompanies {
		if co.ID == companyID {
			return true
		}
	}

	return false
}

func runIndeedFeed() {
	indeedCompanyIntegrations, err := getCompanyIntegrationsUsingIntegrationURL(INDEED_INTEGRATION_URL)
	if err != nil {
		ErrorLog.Printf("err getting INDEED companyIntegrations: %v\n", err)
		return
	}

	indeedEnabledCompanyIntegrations := []CompanyIntegration{}
	for _, ci := range indeedCompanyIntegrations {
		if !checkCompanyBlockedJobsFeeds(ci.CompanyID) && checkIntegrationEnabled(ci) {
			indeedEnabledCompanyIntegrations = append(indeedEnabledCompanyIntegrations, ci)
		}
	}

	zipRecruiterCompanyIntegrations, err := getCompanyIntegrationsUsingIntegrationURL(ZIPRECRUITER_INTEGRATION_URL)
	if err != nil {
		ErrorLog.Printf("err getting ZIPRECRUITER companyIntegrations: %v\n", err)
		return
	}

	zipRecruiterEnabledCompanyIntegrations := []CompanyIntegration{}
	for _, ci := range zipRecruiterCompanyIntegrations {
		if !checkCompanyBlockedJobsFeeds(ci.CompanyID) && checkIntegrationEnabled(ci) {
			zipRecruiterEnabledCompanyIntegrations = append(zipRecruiterEnabledCompanyIntegrations, ci)
		}
	}

	InfoLog.Printf("starting job feed fetch - indeed: %d, ziprecruiter: %d\n", len(indeedEnabledCompanyIntegrations), len(zipRecruiterEnabledCompanyIntegrations))

	// NOTE: if this is initial run, we need to fetch date created which requires
	//       pulling an extra report per company
	fromScratch, err := isStartingFromScratch()

	allJobReqs, err := fetchAllJobReqs(fromScratch, append(indeedEnabledCompanyIntegrations, zipRecruiterEnabledCompanyIntegrations...))
	if err != nil {
		ErrorLog.Printf("INDEED FEED STOPPED: fetchAllJobReqs err: %v\n", err)
		return
	}

	err = findAndSetJobsPublishDates(fromScratch, allJobReqs)
	if err != nil {
		ErrorLog.Printf("INDEED FEED STOPPED: findAndSetJobsPublishDates err: %v\n", err)
		return
	}

	companyJobsMap := putJobsInMapByCompanyID(allJobReqs)

	InfoLog.Printf("starting indeed transformation & upload\n")

	indeedReadyStruct, err := jobReqDetailToIndeedFormat(indeedEnabledCompanyIntegrations, companyJobsMap)
	if err != nil {
		ErrorLog.Printf("INDEED FEED STOPPED: jobReqDetailToIndeedFormat err: %v\n", err)
		return
	}

	indeedFileName := "allclients.xml"
	if !env.Production {
		indeedFileName = "allclients-test.xml"
	}

	err = uploadJobFeed(indeedReadyStruct, indeedFileName, INDEED_FEED_BUCKET_NAME)
	if err != nil {
		ErrorLog.Printf("INDEED FEED UPLOAD FAILED: %v\n", err)
	}

	InfoLog.Printf("starting ziprecruiter ORGANIC transformation & upload\n")

	zrReadyStruct, err := jobReqDetailToZiprecruiterFormat(false, zipRecruiterEnabledCompanyIntegrations, companyJobsMap)
	if err != nil {
		ErrorLog.Printf("ZIPRECRUITER FEED STOPPED: jobReqDetailToZiprecruiterFormat err: %v\n", err)
		return
	}

	zrFileName := "ziprecruiter-organic.xml"
	if !env.Production {
		zrFileName = "ziprecruiter-organic-test.xml"
	}

	err = uploadJobFeed(zrReadyStruct, zrFileName, INDEED_FEED_BUCKET_NAME)
	if err != nil {
		ErrorLog.Printf("ZIPRECRUITER FEED UPLOAD FAILED: %v\n", err)
	}

	InfoLog.Printf("starting ziprecruiter SPONSORED transformation & upload\n")

	zrSponsoredReadyStruct, err := jobReqDetailToZiprecruiterFormat(true, zipRecruiterEnabledCompanyIntegrations, companyJobsMap)
	if err != nil {
		ErrorLog.Printf("ZIPRECRUITER SPONSORED FEED STOPPED: jobReqDetailToZiprecruiterFormat err: %v\n", err)
		return
	}

	zrSponsoredFileName := "ziprecruiter-sponsored.xml"
	if !env.Production {
		zrSponsoredFileName = "ziprecruiter-sponsored-test.xml"
	}

	err = uploadJobFeed(zrSponsoredReadyStruct, zrSponsoredFileName, INDEED_FEED_BUCKET_NAME)
	if err != nil {
		ErrorLog.Printf("ZIPRECRUITER SPONSORED FEED UPLOAD FAILED: %v\n", err)
	}

	InfoLog.Printf("jobsite uploads finished\n")
}

func uploadJobFeed(jobFeedXMLStruct interface{}, fileName, gcpBucketName string) error {
	absPath := fmt.Sprintf("/etc/opintegrations/jobfiles/%s", fileName)
	if !env.Production {
		absPath, _ = filepath.Abs(fmt.Sprintf("./opintegrations/jobfiles/%s", fileName))
	}

	jobFile, err := os.Create(absPath)
	if err != nil {
		return errors.New(fmt.Sprintf("err creating file: %v\n", err))
	}
	defer jobFile.Close()

	headerBytes := []byte(xml.Header)
	_, err = jobFile.Write(headerBytes)
	if err != nil {
		ErrorLog.Printf("header write error: %v\n", err)
	}

	enc := xml.NewEncoder(jobFile)
	enc.Indent("  ", "    ")
	if err := enc.Encode(&jobFeedXMLStruct); err != nil {
		return errors.New(fmt.Sprintf("Encode error: %v\n", err))
	}

	jobFileOpen, err := os.Open(absPath)
	if err != nil {
		return errors.New(fmt.Sprintf("err OPENING file: %v\n", err))
	}
	defer jobFileOpen.Close()

	err = uploadFileGCM(gcpBucketName, fileName, jobFileOpen)
	if err != nil {
		return errors.New(fmt.Sprintf("err uploadFile: %v\n", err))
	}

	return nil
}

func jobReqDetailToZiprecruiterFormat(sponsoredFeed bool, companyInts []CompanyIntegration, companyJobsMap map[int64][]JobReqJoin) (ZRXMLSource, error) {
	source := ZRXMLSource{
		Publisher:     "OnePoint HCM",
		PublisherURL:  "https://onepointhcm.com",
		LastBuildDate: time.Now().Format("Mon, 2 Jan 2006 15:04:05 MST"),
	}

	zrJobs := []ZRXMLJob{}
	for _, companyInt := range companyInts {
		jobReqs, found := companyJobsMap[companyInt.CompanyID]
		if !found {
			continue
		}

		for _, jobReq := range jobReqs {
			pubDate, err := time.Parse("01/02/2006", jobReq.PublishDateInfo.PublishDate)
			if err != nil {
				ErrorLog.Printf("err parsing pubDate (%s) from OP: %+v", jobReq.PublishDateInfo.PublishDate, err)
				pubDate = time.Now()
			}

			referenceID := fmt.Sprintf("%d%s", jobReq.JobReqAPIDetail.ID, pubDate.Format("010206"))

			// Required. The two-letter ISO country code where the position is located
			// OnePoint API does not send two letter so for now hard code
			country := "US"

			// entry Entry Level (0-2 years)
			// mid Mid Level (3-6 years)
			// senior Senior Level (7+ years)
			experience := ""
			if jobReq.JobReqAPIDetail.MinExperience != 0 {
				if jobReq.JobReqAPIDetail.MaxExperience != 0 {
					// both
					if jobReq.JobReqAPIDetail.MinExperience <= 2 {
						experience = "entry"
					} else if jobReq.JobReqAPIDetail.MinExperience <= 6 {
						experience = "mid"
					} else {
						experience = "senior"
					}
				} else {
					// only min
					if jobReq.JobReqAPIDetail.MinExperience <= 2 {
						experience = "entry"
					} else if jobReq.JobReqAPIDetail.MinExperience <= 6 {
						experience = "mid"
					} else {
						experience = "senior"
					}
				}
			} else if jobReq.JobReqAPIDetail.MaxExperience != 0 {
				// only max
				if jobReq.JobReqAPIDetail.MaxExperience <= 2 {
					experience = "entry"
				} else if jobReq.JobReqAPIDetail.MaxExperience <= 6 {
					experience = "mid"
				} else {
					experience = "senior"
				}
			}

			// Onepoint Values: "", "NONE", "HIGH_SCHOOL", "TWO_YEAR_DEGREE", "FOUR_YEAR_DEGREE", "MASTER", "DOCTORATE"
			// ZR Values: "ged", "assoc", "undergrad", "grad"
			education := ""
			switch jobReq.JobReqAPIDetail.RequiredDegree {
			case "HIGH_SCHOOL":
				education = "ged"
			case "TWO_YEAR_DEGREE":
				education = "assoc"
			case "FOUR_YEAR_DEGREE":
				education = "undergrad"
			case "MASTER", "DOCTORATE":
				education = "grad"
			}

			// Acceptable values are Annually, Monthly, Weekly, Daily, and Hourly.
			compInterval := ""
			switch jobReq.JobReqAPIDetail.BasePayFrequency {
			case "YEAR":
				compInterval = "Annually"
			case "MONTH":
				compInterval = "Monthly"
			case "WEEK":
				compInterval = "Weekly"
			case "HOUR":
				compInterval = "Hourly"
			}

			compFrom, compTo := "", ""
			if jobReq.JobReqAPIDetail.BasePayFrom != 0 {
				compFrom = strconv.FormatFloat(jobReq.JobReqAPIDetail.BasePayFrom, 'f', 2, 64)
			}
			if jobReq.JobReqAPIDetail.BasePayTo != 0 {
				compTo = strconv.FormatFloat(jobReq.JobReqAPIDetail.BasePayTo, 'f', 2, 64)
			}

			// If present, with any content other than 0, adds the label "Plus Commission"
			compHasCommission := "0"
			if jobReq.JobReqAPIDetail.AverageCommission != 0 {
				compHasCommission = "yes"
			}

			sponsorship, err := getJobsActiveZRSponsorship(strconv.Itoa(jobReq.JobReqAPIBasic.ID))

			var sponsored *string
			if sponsoredFeed {
				if err == nil {
					sponsored = &sponsorship.Level
				} else {
					// not sponsored job, does not belong in this SPONSORED feed
					continue
				}
			} else {
				if err == nil {
					// a sponsored job, does not belong in this ORGANIC feed
					continue
				}
			}

			descriptionWithLink := addApplyLinkToDescription(jobReq.JobReqAPIDetail.JobDescription, jobReq.CompanyShortName, strconv.Itoa(jobReq.JobReqAPIDetail.ID), "ZipRecruiter")

			jobtype := zipRecruiterJobTypeMap(jobReq.JobReqAPIDetail.EmployeeType.Name)

			iJob := ZRXMLJob{
				ReferenceNumber:           referenceID,
				Title:                     jobReq.JobReqAPIDetail.JobTitle,
				Description:               ZRDescription{Text: descriptionWithLink},
				Country:                   country,
				City:                      jobReq.JobReqAPIDetail.Location.City,
				State:                     jobReq.JobReqAPIDetail.Location.State,
				PostalCode:                jobReq.JobReqAPIDetail.Location.Zip,
				Company:                   jobReq.CompanyName,
				Date:                      pubDate.Format("Mon, 2 Jan 2006 15:04:05 MST"),
				URL:                       ZRURL{Text: fmt.Sprintf("https://secure.onehcm.com/ta/%s.jobs?ShowJob=%s&TrackId=ZipRecruiter", jobReq.CompanyShortName, strconv.Itoa(jobReq.JobReqAPIDetail.ID))},
				Experience:                experience,
				Education:                 education,
				CompensationInterval:      compInterval,
				CompensationMin:           compFrom,
				CompensationMax:           compTo,
				CompensationHasCommission: compHasCommission,
				Sponsored:                 sponsored,
				JobType:                   jobtype,
			}

			zrJobs = append(zrJobs, iJob)
		}

	}

	source.Jobs = zrJobs

	return source, nil
}

func jobReqDetailToIndeedFormat(companyInts []CompanyIntegration, companyJobsMap map[int64][]JobReqJoin) (IndeedXMLSource, error) {
	source := IndeedXMLSource{
		Publisher:     "OnePoint HCM",
		PublisherURL:  "https://onepointhcm.com",
		LastBuildDate: time.Now().Format("Mon, 2 Jan 2006 15:04:05 MST"),
	}

	iJobs := []IndeedXMLJob{}
	for _, companyInt := range companyInts {
		jobReqs, found := companyJobsMap[companyInt.CompanyID]
		if !found {
			continue
		}

		for _, jobReq := range jobReqs {
			pubDate, err := time.Parse("01/02/2006", jobReq.PublishDateInfo.PublishDate)
			if err != nil {
				ErrorLog.Printf("err parsing pubDate (%s) from OP: %+v", jobReq.PublishDateInfo.PublishDate, err)
				pubDate = time.Now()
			}

			payFrom, payTo := "", ""
			if jobReq.JobReqAPIDetail.BasePayFrom != 0 {
				payFrom = strconv.FormatFloat(jobReq.JobReqAPIDetail.BasePayFrom, 'f', 2, 64)
			}
			if jobReq.JobReqAPIDetail.BasePayTo != 0 {
				payTo = strconv.FormatFloat(jobReq.JobReqAPIDetail.BasePayTo, 'f', 2, 64)
			}

			salary := ""
			if payFrom != "" {
				if payTo != "" && !(payTo != payFrom) {
					salary = fmt.Sprintf("%s to %s", payFrom, payTo)
				} else {
					salary = fmt.Sprintf("%s", payFrom)
				}
			} else if payTo != "" {
				salary = fmt.Sprintf("%s", payTo)
			}
			if salary != "" {
				salary = salary + " per " + strings.ToLower(jobReq.JobReqAPIDetail.BasePayFrequency)
			}

			expFrom, expTo := "", ""
			if jobReq.JobReqAPIDetail.MinExperience != 0 {
				expFrom = strconv.Itoa(jobReq.JobReqAPIDetail.MinExperience)
			}
			if jobReq.JobReqAPIDetail.MaxExperience != 0 {
				expTo = strconv.Itoa(jobReq.JobReqAPIDetail.MaxExperience)
			}
			experience := ""
			if expFrom != "" {
				if expTo != "" {
					experience = fmt.Sprintf("%s to %s", expFrom, expTo)
				} else {
					experience = fmt.Sprintf("%s", expFrom)
				}
			} else if expTo != "" {
				experience = fmt.Sprintf("%s", expTo)
			}
			if experience != "" {
				if experience == "1" {
					experience = experience + " year"
				} else {
					experience = experience + " years"
				}
			}

			sponsorship, err := getJobsActiveIndeedSponsorship(strconv.Itoa(jobReq.JobReqAPIBasic.ID))

			sponsored := "no"
			budget := ""
			phone := ""
			contactName := ""
			if err == nil {
				sponsored = "yes"
				budget = sponsorship.Budget
				phone = sponsorship.Phone
				contactName = sponsorship.ContactName
			}

			referenceID := fmt.Sprintf("%d%s", jobReq.JobReqAPIDetail.ID, pubDate.Format("010206"))

			descriptionWithLink := addApplyLinkToDescription(jobReq.JobReqAPIDetail.JobDescription, jobReq.CompanyShortName, strconv.Itoa(jobReq.JobReqAPIDetail.ID), "Indeed")

			iaData := url.Values{}
			iaData.Add("indeed-apply-jobTitle", jobReq.JobReqAPIDetail.JobTitle)
			iaData.Add("indeed-apply-jobId", referenceID)
			iaData.Add("indeed-apply-jobUrl", fmt.Sprintf("https://secure.onehcm.com/ta/%s.jobs?ShowJob=%d&TrackId=Indeed", jobReq.CompanyShortName, jobReq.JobReqAPIDetail.ID))
			iaData.Add("indeed-apply-jobCompanyName", jobReq.CompanyName)
			iaData.Add("indeed-apply-apiToken", INDEED_API_TOKEN)
			iaData.Add("indeed-apply-postUrl", fmt.Sprintf("https://connect.onehcm.com/api/recruitment/application?company_shortname=%s&job_id=%d&source=indeed", jobReq.CompanyShortName, jobReq.JobReqAPIDetail.ID))
			iaData.Add("indeed-apply-jobLocation", fmt.Sprintf("%s, %s %s", jobReq.JobReqAPIDetail.Location.City, jobReq.JobReqAPIDetail.Location.State, jobReq.JobReqAPIDetail.Location.Zip))
			iaData.Add("indeed-apply-resume", "required")
			iaData.Add("indeed-apply-coverletter", "hidden")

			iJob := IndeedXMLJob{
				Title:           IndeedTitle{Text: jobReq.JobReqAPIDetail.JobTitle},
				Date:            IndeedDate{Text: pubDate.Format("Mon, 2 Jan 2006 15:04:05 MST")},
				ReferenceNumber: IndeedReferenceNumber{Text: referenceID},
				URL:             IndeedURL{Text: fmt.Sprintf("https://secure.onehcm.com/ta/%s.jobs?ShowJob=%s&TrackId=Indeed", jobReq.CompanyShortName, strconv.Itoa(jobReq.JobReqAPIDetail.ID))},
				Company:         IndeedCompany{Text: jobReq.CompanyName},
				SourceName:      IndeedSourceName{Text: jobReq.CompanyName},
				City:            IndeedCity{Text: jobReq.JobReqAPIDetail.Location.City},
				State:           IndeedState{Text: jobReq.JobReqAPIDetail.Location.State},
				Country:         IndeedCountry{Text: jobReq.JobReqAPIDetail.Location.Country},
				PostalCode:      IndeedPostalCode{Text: jobReq.JobReqAPIDetail.Location.Zip},
				Description:     IndeedDescription{Text: descriptionWithLink},
				Salary:          IndeedSalary{Text: salary},
				Education:       IndeedEducation{Text: jobReq.JobReqAPIDetail.RequiredDegree},
				Category:        IndeedCategory{Text: strings.Join(jobReq.JobReqAPIDetail.JobCategories, ", ")},
				Experience:      IndeedExperience{Text: experience},
				IndeedApplyData: IndeedApplyData{Text: iaData.Encode()},
				Email:           IndeedEmail{Text: jobReq.ContactEmail},
				Sponsored:       IndeedSponsored{Text: sponsored},
				Budget:          IndeedBudget{Text: budget},
				Phone:           IndeedPhone{Text: phone},
				ContactName:     IndeedContactName{Text: contactName},
			}

			iJobs = append(iJobs, iJob)
		}
	}

	source.Jobs = iJobs

	return source, nil
}

func addApplyLinkToDescription(description, companyShortName, jobReqID, sourceTrackID string) string {
	link := fmt.Sprintf("https://secure.onehcm.com/ta/%s.jobs?ShowJob=%s&TrackId=%s", companyShortName, jobReqID, sourceTrackID)

	descriptionFormated := `<p><b>Apply Here:</b> %s</p>%s`

	return fmt.Sprintf(descriptionFormated, link, description)
}

func getJobsActiveIndeedSponsorship(opID string) (sponsorship IndeedSponsorship, err error) {
	now := time.Now().Unix()
	qry := `SELECT * FROM indeed_sponsorships WHERE op_job_id = ? AND time_end > ? AND manually_ended = 0`
	err = dbmap.SelectOne(&sponsorship, qry, opID, now)
	return
}

func getJobsActiveZRSponsorship(opID string) (sponsorship ZipRecruiterSponsorship, err error) {
	now := time.Now().Unix()
	qry := `SELECT * FROM ziprecruiter_sponsorships WHERE op_job_id = ? AND time_end > ? AND manually_ended = 0`
	err = dbmap.SelectOne(&sponsorship, qry, opID, now)
	return
}

func putJobsInMapByCompanyID(jobReqs []JobReqJoin) map[int64][]JobReqJoin {
	companyJobsMap := make(map[int64][]JobReqJoin)

	for _, job := range jobReqs {
		existingArr, exists := companyJobsMap[job.CompanyID]
		if exists {
			existingArr = append(existingArr, job)
			companyJobsMap[job.CompanyID] = existingArr
		} else {
			companyJobsMap[job.CompanyID] = []JobReqJoin{job}
		}
	}

	return companyJobsMap
}

// ZR has strict types - OP can define own type names
// ZR TYPES: Full-Time, Part-Time, Contractor, Temporary, Other
func zipRecruiterJobTypeMap(opJobType string) string {
	if opJobType == "Full-Time" || opJobType == "Part-Time" || opJobType == "Contractor" || opJobType == "Temporary" || opJobType == "Other" {
		return opJobType
	}

	loweredOPType := strings.ToLower(opJobType)

	if strings.Contains(loweredOPType, "full") || strings.Contains(loweredOPType, "ft") {
		return "Full-Time"
	}

	// problem is "exempt" and "non exempt" both have "pt" - problem if "EXEMPT" etc
	if strings.Contains(loweredOPType, "part") || strings.Contains(opJobType, "PT") {
		return "Part-Time"
	}

	return ""
}
