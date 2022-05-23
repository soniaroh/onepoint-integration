package main

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"opapi"
)

type CompanyJobReqsReport struct {
	Active               string `csv:"Active"`
	Budgeted             string `csv:"Budgeted"`
	JobStatus            string `csv:"Job Status"`
	RequisitionID        string `csv:"Requisition #"`
	Location             string `csv:"Location"`
	JobTitle             string `csv:"Job Title"`
	JobCategory          string `csv:"Job Category"`
	JobIndustry          string `csv:"Job Industry #1"`
	ContactName          string `csv:"Contact Name"`
	ContactPhone         string `csv:"Contact Phone"`
	ContactEmail         string `csv:"Contact Email"`
	Created              string `csv:"Created"`
	ApproverFirstName    string `csv:"Approver First Name"`
	ApproverLastName     string `csv:"Approver Last Name"`
	WorkflowStatus       string `csv:"Workflow Status"`
	VisibilityType       string `csv:"Visibility Type"`
	DateToPostExternally string `csv:"Date To Post Externally"`
	EQuestRequested      string `csv:"E-Quest requested"`
}

type JobReqJoin struct {
	JobReqAPIBasic      opapi.JobReqBasic
	JobReqAPIDetail     opapi.JobReqDetail
	DateCreatedOPFormat string
	CompanyShortName    string
	CompanyName         string
	ContactEmail        string
	CompanyID           int64
	CompanyOPID         int64
	PublishDateInfo     CurrentJobReq
}

type CurrentJobReq struct {
	ID          int64  `db:"id, primarykey, autoincrement"`
	OPID        int64  `db:"op_id" json:"op_id"`
	CompanyOPID int64  `db:"company_op_id" json:"company_op_id"`
	PublishDate string `db:"publish_date" json:"publish_date"`
}

func fetchAllJobReqs(fromScratch bool, companyIntegrations []CompanyIntegration) ([]JobReqJoin, error) {
	allJoinedJobReqs := []JobReqJoin{}
	fetchedCompanies := make(map[int64]bool)

	successfulCompanies := 0
	successfulJobs := 0
	for _, companyInt := range companyIntegrations {
		_, fetched := fetchedCompanies[companyInt.CompanyID]
		if fetched {
			continue
		}

		fetchedCompanies[companyInt.CompanyID] = true

		company, _ := lookupCompanyByID(companyInt.CompanyID)

		opCxn := chooseOPAPICxn(company.OPID)

		jobsProd, _ := getCompanyProductByURL(company.ID, "jobs")
		emailStr, _ := jobsProd.Settings["contact_email"].(string)

		jrj := JobReqJoin{
			CompanyName:      company.Name,
			CompanyShortName: company.ShortName,
			ContactEmail:     emailStr,
			CompanyID:        company.ID,
			CompanyOPID:      company.OPID,
		}

		// NOTE: retries needed because OP API sometimes fails for no reason
		jobReqBasics, err := fetchCompanyJobsWithRetries(1, opCxn, company.ShortName)
		if err != nil {
			ErrorLog.Printf("err fetchCompanyJobsWithRetries. company: %s, err: %v\n", company.ShortName, err)
			continue
		}

		// NOTE: needed because the API endpoint does not give date created,
		// but indeed requires it
		opIDtoDateCreatedMap := make(map[string]string)
		if fromScratch {
			jobReqsFromReport, err := fetchCompanyJobsFromReport(opCxn, company.ShortName)
			if err != nil {
				ErrorLog.Printf("err fetchCompanyJobsFromReport company jobs. company: %s, err: %v\n", company.ShortName, err)
				continue
			}

			opIDtoDateCreatedMap = generateJobIDtoDateCreatedMap(jobReqsFromReport)
		}

		for _, jobReq := range jobReqBasics {
			// NOTE: retries needed because OP API sometimes fails for no reason
			jobReqDetail, err := fetchJobDetailWithRetries(1, opCxn, company.ShortName, strconv.Itoa(jobReq.ID))
			if err != nil {
				ErrorLog.Printf("err fetching job detail. company: %s, jobreqid: %v, err: %v\n", company.ShortName, strconv.Itoa(jobReq.ID), err)
				continue
			}

			if fromScratch {
				dateCreated, ok := opIDtoDateCreatedMap[strconv.Itoa(jobReq.ID)]
				if ok {
					jrj.DateCreatedOPFormat = dateCreated
				}
			}

			jrj.JobReqAPIDetail = jobReqDetail
			jrj.JobReqAPIBasic = jobReq
			allJoinedJobReqs = append(allJoinedJobReqs, jrj)

			jrj.DateCreatedOPFormat = ""

			successfulJobs++
		}

		successfulCompanies++
	}

	InfoLog.Printf("done fetching - successful company fetches: %v, jobs: %v\n", successfulCompanies, successfulJobs)

	return allJoinedJobReqs, nil
}

func findAndSetJobsPublishDates(fromScratch bool, jobs []JobReqJoin) error {
	hundredTwentyDaysAgo := time.Now().AddDate(0, 0, -120)

	if fromScratch {
		InfoLog.Println("findAndSetJobsPublishDates initial run")
		// this is the initial run, we must use date created per indeed's request
		// if date created is < 120 days ago use, else use today
		for index := 0; index < len(jobs); index++ {
			dateCreated, err := time.Parse("01/02/2006 03:04pm", jobs[index].DateCreatedOPFormat+"m")
			if err != nil {
				dateCreated = time.Now()
			}

			dateToUse := dateCreated

			if dateCreated.Unix() < hundredTwentyDaysAgo.Unix() {
				dateToUse = time.Now()
			}

			currentJob := CurrentJobReq{
				OPID:        int64(jobs[index].JobReqAPIDetail.ID),
				PublishDate: dateToUse.Format("01/02/2006"),
				CompanyOPID: jobs[index].CompanyOPID,
			}

			jobs[index].PublishDateInfo = currentJob
		}
	} else {
		for index := 0; index < len(jobs); index++ {
			existingJob := CurrentJobReq{}
			err := dbmap.SelectOne(&existingJob, "SELECT * FROM current_job_reqs WHERE op_id = ?", jobs[index].JobReqAPIDetail.ID)

			if err != nil {
				existingJob.OPID = int64(jobs[index].JobReqAPIDetail.ID)
				existingJob.PublishDate = time.Now().Format("01/02/2006")
				existingJob.CompanyOPID = jobs[index].CompanyOPID
			} else {
				// if exists use that date but check if need to reset
				oldPubDate, err := time.Parse("01/02/2006", existingJob.PublishDate)
				if err != nil {
					ErrorLog.Println("couldnt parse date of job: " + existingJob.PublishDate)
					existingJob.PublishDate = time.Now().Format("01/02/2006")
				}

				if oldPubDate.Unix() < hundredTwentyDaysAgo.Unix() {
					existingJob.PublishDate = time.Now().Format("01/02/2006")
				}
			}

			jobs[index].PublishDateInfo = existingJob
		}
	}

	// delete all and replace with most recent
	_, err := dbmap.Exec("TRUNCATE TABLE current_job_reqs")
	if err != nil {
		return errors.New("could not truncate table so could not update table")
	}

	for index := 0; index < len(jobs); index++ {
		err = dbmap.Insert(&jobs[index].PublishDateInfo)
		if err != nil {
			ErrorLog.Println("inserting pub date err: " + err.Error())
		}
	}

	return nil
}

func generateJobIDtoDateCreatedMap(jobReqsFromReport []CompanyJobReqsReport) map[string]string {
	m := make(map[string]string)

	for _, jobReq := range jobReqsFromReport {
		m[jobReq.RequisitionID] = jobReq.Created
	}

	return m
}

func fetchCompanyJobsFromReport(opCxn *opapi.OPConnection, companyShort string) (jobs []CompanyJobReqsReport, err error) {
	err = opCxn.GenricReport(fmt.Sprintf("https://secure.onehcm.com/ta/rest/v1/report/global/REPORT_HR_JOB_REQUISITIONS?company:shortname=%s", companyShort), &jobs)
	return jobs, err
}

func fetchJobDetailWithRetries(maxAttempts int, opCxn *opapi.OPConnection, companyShort, jobID string) (opapi.JobReqDetail, error) {
	attempts := 0
	jobReqDetail := opapi.JobReqDetail{}
	var err error

	for {
		if attempts == maxAttempts {
			return jobReqDetail, errors.New(fmt.Sprintf("err fetchJobDetailWithRetries company jobs NO MORE ATTEMPTS %s", companyShort))
		}

		jobReqDetail, err = opCxn.GetDetailedJobReqByID(companyShort, jobID)
		if err == nil {
			return jobReqDetail, nil
		} else {
			if err.Error() == "Job Requisition is no longer active." {
				return jobReqDetail, err
			}
		}

		attempts++
	}
}

func fetchCompanyJobsWithRetries(maxAttempts int, opCxn *opapi.OPConnection, companyShort string) ([]opapi.JobReqBasic, error) {
	attempts := 0
	jobReqBasics := []opapi.JobReqBasic{}
	var err error

	for {
		if attempts == maxAttempts {
			return jobReqBasics, errors.New(fmt.Sprintf("err fetchCompanyJobs company jobs NO MORE ATTEMPTS %s", companyShort))
		}

		jobReqBasics, err = fetchCompanyJobs(opCxn, companyShort)
		if err == nil {
			return jobReqBasics, nil
		}

		attempts++
	}
}

func fetchCompanyJobs(opCxn *opapi.OPConnection, companyShort string) ([]opapi.JobReqBasic, error) {
	cjr, err := opCxn.GetCompanyJobReqs(companyShort)
	return cjr.JobRequisitions, err
}

func isStartingFromScratch() (bool, error) {
	countExistingCurrentJobs, err := dbmap.SelectInt("SELECT COUNT(*) FROM current_job_reqs")
	if err != nil {
		return false, errors.New("bad initial query: " + err.Error())
	}

	if countExistingCurrentJobs == 0 {
		return true, nil
	}

	return false, nil
}
