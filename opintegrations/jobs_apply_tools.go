package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"opapi"
)

type ApplyStats struct {
	JBApplied        int    `json:"jb_applied"`
	OPApplied        int    `json:"op_applied"`
	PercentConverted string `json:"percent_converted"`
}

type JobApplicationsReport struct {
	RequisitionID                string `csv:"Requisition #"`
	Location                     string `csv:"Location"`
	JobTitle                     string `csv:"Job Title"`
	JobRequisitionEIN            string `csv:"Job Requisition EIN"`
	JobCategory                  string `csv:"Job Category"`
	JobIndustry                  string `csv:"Job Industry #1"`
	FirstName                    string `csv:"First Name"`
	LastName                     string `csv:"Last Name"`
	Email                        string `csv:"Email"`
	ApplicationStatus            string `csv:"Application Status"`
	JobApplicationHiringStage    string `csv:"Job Application Hiring Stage"`
	JobStatus                    string `csv:"Job Status"`
	CoverLetter                  string `csv:"Cover Letter / Additional Comment"`
	AppliedOn                    string `csv:"Applied On"`
	ApplicationRank              string `csv:"Application Rank"`
	ApplicantQuestionnaireStatus string `csv:"Applicant Questionnaire Status"`
}

type CompanyWithApplyStats struct {
	CompanyName string     `json:"company"`
	ApplyStats  ApplyStats `json:"stats"`
}

type ApplyStatsReturnBody struct {
	OverallJBApplied        int                     `json:"overall_jb_applied"`
	OverallOPApplied        int                     `json:"overall_op_applied"`
	OverallPercentConverted string                  `json:"overall_percent_converted"`
	CoData                  []CompanyWithApplyStats `json:"companies"`
}

func runApplyFollowUps() {
	cos, err := findCompaniesWithAtLeastOneActiveIntegrationInProduct("jobs")
	if err != nil {
		ErrorLog.Println("findCompaniesWithAtLeastOneActiveIntegrationInProduct failed: " + err.Error())
	}

	for _, co := range cos {
		InfoLog.Println("~~~~~~~~~ follow ups starting company: " + co.ShortName)
		err = updateAppliedStatus(co)
		if err != nil {
			ErrorLog.Println("~~~~~~~~~ FAILED: " + err.Error())
		}
	}
}

func runApplyStats() (CompanyWithApplyStats, []CompanyWithApplyStats, error) {
	allStats := []CompanyWithApplyStats{}
	overallStats := CompanyWithApplyStats{}

	cos, err := findCompaniesWithAtLeastOneActiveIntegrationInProduct("jobs")
	if err != nil {
		return overallStats, allStats, errors.New("findCompaniesWithAtLeastOneActiveIntegrationInProduct failed: " + err.Error())
	}

	sort.Sort(AlphaByName(cos))

	totalAppliedOnJB := 0
	totalAppliedOnOP := 0

	for _, co := range cos {
		InfoLog.Println("~~~~~~~~~ follow ups starting company: " + co.ShortName)
		stats, err := findAppliedStats(co.ID)
		if err != nil {
			ErrorLog.Println("~~~~~~~~~ FAILED: " + err.Error())
			continue
		}

		totalAppliedOnJB += stats.JBApplied
		totalAppliedOnOP += stats.OPApplied

		allStats = append(allStats, CompanyWithApplyStats{CompanyName: co.Name, ApplyStats: stats})
	}

	overallStats.ApplyStats.JBApplied = totalAppliedOnJB
	overallStats.ApplyStats.OPApplied = totalAppliedOnOP

	totalAppliedOnJBF := float64(totalAppliedOnJB)
	totalAppliedOnOPF := float64(totalAppliedOnOP)

	overallStats.ApplyStats.PercentConverted = fmt.Sprintf("%.2f%%", (totalAppliedOnOPF/totalAppliedOnJBF)*100)

	return overallStats, allStats, nil
}

func updateAppliedStatus(thisCompany Company) error {
	cxn := chooseOPAPICxn(thisCompany.OPID)

	// get all apps applied_on_op = 0
	allUnAppliedApps := []JobApplication{}
	_, err := dbmap.Select(&allUnAppliedApps, "SELECT * FROM job_applications WHERE company_id = ? AND (applied_on_op IS NULL OR applied_on_op = 0)", thisCompany.ID)
	if err != nil {
		return errors.New(fmt.Sprintf("Job apps lookup err: %v", err))
	}

	// get all the OP apps
	allOPApps, err := getAllOPJobApps(cxn, thisCompany.ShortName)
	if err != nil {
		return errors.New(fmt.Sprintf("getAllOPJobApps err: %v", err))
	}

	// make map[string][]string of applicant email to job req id that they have applied for
	opApplicantToJobsAppliedFor := makeApplicantToJobAppsMap(allOPApps)

	// go through OPC list of apps, looking up applicants in map
	// 	if applied for job, mark true and update app
	// 	if not applied, if applied time is > 24 hours ago (and < a week ago) AND we have not already follow up, send follow up email and update
	for _, jobBoardApp := range allUnAppliedApps {
		appliedOnOPForThisJob := false
		jobsArr, exists := opApplicantToJobsAppliedFor[strings.ToLower(jobBoardApp.ApplicantEmail)]
		if exists {
			for _, job := range jobsArr {
				if job == jobBoardApp.OPJobID {
					appliedOnOPForThisJob = true
				}
			}
		}

		if appliedOnOPForThisJob {
			// update to applied
			InfoLog.Printf("APPLIED! email: %s, jobid: %s\n", jobBoardApp.ApplicantEmail, jobBoardApp.OPJobID)
			if env.Production {
				jobBoardApp.AppliedOnOP = &appliedOnOPForThisJob
				_, err = dbmap.Update(&jobBoardApp)
				if err != nil {
					ErrorLog.Println("could not update job app to indicate applied: " + err.Error())
				}
			}
		}

		if !exists || !appliedOnOPForThisJob {
			// never applied
			timeNowMillis := time.Now().Unix() * 1000
			millisecondsInDay := 86400000
			epochTwentyFoursAgo := timeNowMillis - int64(millisecondsInDay)
			epochAWeekAgo := timeNowMillis - int64(7*millisecondsInDay)

			if epochAWeekAgo < jobBoardApp.AppliedOnMillis && jobBoardApp.AppliedOnMillis < epochTwentyFoursAgo {
				if jobBoardApp.FollowUpEmailSent == nil || *jobBoardApp.FollowUpEmailSent == false {
					if env.Production {
						err = sendJobAppFinishEmail(&jobBoardApp, thisCompany, true)
						if err != nil {
							ErrorLog.Println("err sending reminder email: " + err.Error())
						} else {
							tru := true
							jobBoardApp.FollowUpEmailSent = &tru
							_, err = dbmap.Update(&jobBoardApp)
							if err != nil {
								ErrorLog.Println("err sending reminder email BUT EMAIL DID SEND !!!!: " + err.Error())
							} else {
								InfoLog.Println("sent follow up email and updated successfully, email: " + jobBoardApp.ApplicantEmail)
							}
						}
					} else {
						InfoLog.Printf("would be emailing: email: %s, jobid: %s, applied on %d\n", jobBoardApp.ApplicantEmail, jobBoardApp.OPJobID, jobBoardApp.AppliedOnMillis)
					}
				}
			}
		}
	}

	return nil
}

func findAppliedStats(companyID int64) (ApplyStats, error) {
	stats := ApplyStats{}

	thisCompany, err := lookupCompanyByID(companyID)
	if err != nil {
		return stats, errors.New(fmt.Sprintf("Could not lookup company id: %d", companyID))
	}

	cxn := chooseOPAPICxn(thisCompany.OPID)

	// get all apps
	allAppliedApps := []JobApplication{}
	_, err = dbmap.Select(&allAppliedApps, "SELECT * FROM job_applications WHERE company_id = ?", companyID)
	if err != nil {
		return stats, errors.New(fmt.Sprintf("Job apps lookup err: %v", err))
	}

	countAppliedJB := len(allAppliedApps)

	if countAppliedJB == 0 {
		return ApplyStats{PercentConverted: "n/a"}, nil
	}

	// get all the OP apps
	allOPApps, err := getAllOPJobApps(cxn, thisCompany.ShortName)
	if err != nil {
		return stats, errors.New(fmt.Sprintf("getAllOPJobApps err: %v", err))
	}

	// make map[string][]string of applicant email to job req id that they have applied for
	opApplicantToJobsAppliedFor := makeApplicantToJobAppsMap(allOPApps)

	countConvertedToOP := 0

	for _, jobBoardApp := range allAppliedApps {
		appliedOnOPForThisJob := false
		jobsArr, exists := opApplicantToJobsAppliedFor[strings.ToLower(jobBoardApp.ApplicantEmail)]
		if exists {
			for _, job := range jobsArr {
				if job == jobBoardApp.OPJobID {
					appliedOnOPForThisJob = true
				}
			}
		}

		if appliedOnOPForThisJob {
			countConvertedToOP++
		}
	}

	stats.OPApplied = countConvertedToOP
	stats.JBApplied = countAppliedJB

	countConvertedToOPF := float64(countConvertedToOP)
	countJBAppliedF := float64(countAppliedJB)
	stats.PercentConverted = fmt.Sprintf("%.2f%%", (countConvertedToOPF/countJBAppliedF)*100)

	return stats, nil
}

func makeApplicantToJobAppsMap(allOPApps []JobApplicationsReport) map[string][]string {
	opApplicantToJobsAppliedFor := make(map[string][]string)
	for _, opApplication := range allOPApps {
		applicantEmailLowered := strings.ToLower(opApplication.Email)
		jobsArr, exists := opApplicantToJobsAppliedFor[applicantEmailLowered]
		if exists {
			jobsArr = append(jobsArr, opApplication.RequisitionID)
			opApplicantToJobsAppliedFor[applicantEmailLowered] = jobsArr
		} else {
			opApplicantToJobsAppliedFor[applicantEmailLowered] = []string{opApplication.RequisitionID}
		}
	}

	return opApplicantToJobsAppliedFor
}

func getAllOPJobApps(cxn *opapi.OPConnection, companyShortName string) ([]JobApplicationsReport, error) {
	allOPApps := []JobApplicationsReport{}
	url := fmt.Sprintf("https://secure.onehcm.com/ta/rest/v1/report/global/REPORT_HR_JOB_APPLICATIONS?company:shortname=%s", companyShortName)

	if companyShortName == "HarrisBruno" {
		url = "https://secure.onehcm.com/ta/rest/v1/report/saved/37703710?company:shortname=HARRISBRUNO"
	}

	if companyShortName == "WHINC" {
		url = "https://secure.onehcm.com/ta/rest/v1/report/saved/37703731?company:shortname=WHINC"
	}

	if companyShortName == "SNAHC" {
		url = "https://secure.onehcm.com/ta/rest/v1/report/saved/37754579?company:shortname=SNAHC"
	}

	err := cxn.GenricReport(url, &allOPApps)
	if err != nil {
		return allOPApps, errors.New(fmt.Sprintf("Could not GET report: %v", err))
	}

	return allOPApps, nil
}
