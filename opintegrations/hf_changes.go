package main

import (
	"fmt"
	"time"

	"opapi"
)

func runUserProvisioningChanges() {
	InfoLog.Println("starting runUserProvisioningChanges")

	cis, err := getCompanyIntegrationsUsingIntegrationURL(adIntegrationURL)
	if err != nil {
		ErrorLog.Println("getCompanyIntegrationsUsingIntegrationURL err: " + err.Error())
		return
	}

	for _, ci := range cis {
		if !checkIntegrationEnabled(ci) {
			continue
		}

		company, _ := lookupCompanyByID(ci.CompanyID)

		InfoLog.Println("starting company: " + company.ShortName)

		lastTimeFetched, err := lookupLastTimeFetchedUpdateCreateIfDoesntExist(company.ID)
		if err != nil {
			ErrorLog.Println("lookupLastTimeFetchedUpdateCreateIfDoesntExist err :" + err.Error())
			continue
		}

		timeFetchedString := lastTimeFetched.Format("2006-01-02T15:04:05Z")
		cxn := chooseOPAPICxn(company.OPID)
		changedEmpResponse, err := cxn.GetChangedEmployees(company.OPID, timeFetchedString)
		if err != nil {
			ErrorLog.Println("GetChangedEmployees err :" + err.Error())
			continue
		}

		err = updateLastTimeFetched(company.ID, time.Now().UTC())
		if err != nil {
			ErrorLog.Println("updateLastTimeFetched err :" + err.Error())
			continue
		}

		modifiedCount := 0
		for _, changedEmp := range changedEmpResponse.Entries {
			if changedEmp.Status != "MODIFIED" {
				continue
			}

			// the employee can be pending but they are changed again, we would
			// be adding another event/action and then doing double work, so here we
			// should check that there are no pending events, or we delete the old ones
			// and add this
			if checkExistingPendingChanges(ci, changedEmp.Object) {
				continue
			}

			fullEmployeeRecord, err := cxn.GetFullEmployeeInfo(int64(changedEmp.ID), company.OPID)
			if err != nil {
				ErrorLog.Printf("could not lookup hf employee: %d, err: %v\n", changedEmp.ID, err)
				continue
			}

			adUsername, jobTitle, err := checkHFTemplateMapping(company.OPID, ci, fullEmployeeRecord)
			if err != nil {
				ErrorLog.Println("AD checkHFTemplateMappingBlocked err: ", err)
				continue
			}
			if adUsername == nil {
				reason := "no job"
				if jobTitle != nil {
					reason = fmt.Sprintf("The new employee was given the Job Title of %s, which is not assigned an Active Directory Template.", *jobTitle)
				}
				ErrorLog.Printf("AD changed emp was blocked by cost center, reason: %s\n", reason)
				continue
			}

			modifiedCount++

			processed := true
			newHFEvent := HFEvent{
				Type:        CHANGED_TYPE_ID,
				OPAccountID: int64(changedEmp.ID),
				EventDate:   time.Now().Format("01/02/2006"),
				CompanyID:   ci.CompanyID,
				FirstName:   changedEmp.Object.FirstName,
				LastName:    changedEmp.Object.LastName,
				Email:       changedEmp.Object.Username,
				Processed:   &processed,
				Created:     time.Now().Unix(),
			}

			err = dbmap.Insert(&newHFEvent)
			if err != nil {
				ErrorLog.Println("couldnt Insert new event: ", err)
				continue
			}

			_, err = addHFResult(ci.ID, HF_PENDING_LABEL, newHFEvent.ID, "Waiting for your On-Premise Service to process the employee update.")
			if err != nil {
				ErrorLog.Println("addHFResult err: ", err)
				continue
			}
		}
		InfoLog.Printf("got %d MODIFIEDs using since=%s \n", modifiedCount, timeFetchedString)
	}
}

type HFLastTimeFetchedChanges struct {
	ID          int64 `db:"id, primarykey, autoincrement" json:"id"`
	CompanyID   int64 `db:"company_id" json:"-"`
	LastFetched int64 `db:"last_fetched" json:"-"`
}

func lookupLastTimeFetchedUpdateCreateIfDoesntExist(companyID int64) (time.Time, error) {
	lastFetched := HFLastTimeFetchedChanges{}
	err := dbmap.SelectOne(&lastFetched, "SELECT * FROM hf_last_fetched_changes WHERE company_id = ?", companyID)
	if err != nil {
		lastFetched.CompanyID = companyID
		lastFetched.LastFetched = time.Now().UTC().Unix()
		dbmap.Insert(&lastFetched)
	}

	lastFetchedDate := time.Unix(lastFetched.LastFetched, 0).UTC()

	return lastFetchedDate, nil
}

func updateLastTimeFetched(companyID int64, t time.Time) error {
	lastFetched := HFLastTimeFetchedChanges{}
	err := dbmap.SelectOne(&lastFetched, "SELECT * FROM hf_last_fetched_changes WHERE company_id = ?", companyID)
	if err != nil {
		return err
	}

	lastFetched.LastFetched = t.Unix()

	_, err = dbmap.Update(&lastFetched)
	if err != nil {
		return err
	}

	return nil
}

func checkExistingPendingChanges(ci CompanyIntegration, changedEmp opapi.ChangedEmployeesResponseResult) bool {
	count, err := dbmap.SelectInt("SELECT COUNT(*) from hf_results r, hf_events e WHERE r.company_integration_id = ? AND r.hf_event_id = e.id AND e.op_account_id = ? AND r.status = ? AND e.type = ?", ci.ID, changedEmp.ID, HF_PENDING_LABEL, CHANGED_TYPE_ID)
	if err != nil {
		ErrorLog.Println("checkExistingPendingChanges err: " + err.Error())
	}

	return count > 0
}
