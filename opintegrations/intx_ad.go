package main

import (
	"fmt"

	"opapi"
)

const (
	adIntegrationURL = "activedirectory"
)

// AD starts as pending, waiting OnPremiseService to process and ACK
func adHired(newHiredEvent *HFEvent, opCompanyID int64, fullEmployeeRecord opapi.OPFullEmployeeRecord, resultsSoFar []HFCompanyIntegrationResultJoin) (string, string, HFCreatedCredentials) {
	var createdCredentials = HFCreatedCredentials{}

	thisCompanyIntegration, err := getCompanyIntegrationByIntegrationString(adIntegrationURL, newHiredEvent.CompanyID)
	if err != nil {
		ErrorLog.Println("adHired couldnt look up company integration: ", err)
		return HF_FAILED_LABEL, "An error occured on our end, we will investigate this issue.", createdCredentials
	}

	newHiredEvent.Email = fullEmployeeRecord.Username
	*newHiredEvent.Processed = true
	dbmap.Update(newHiredEvent)

	if !checkIntegrationEnabled(thisCompanyIntegration) {
		InfoLog.Println("new hired but AD disabled")

		return HF_DISABLED_LABEL, "AD Integration was disabled at the time of processing event.", createdCredentials
	}

	adUsername, jobTitle, err := checkHFTemplateMapping(opCompanyID, thisCompanyIntegration, fullEmployeeRecord)
	if err != nil {
		ErrorLog.Println("AD checkHFTemplateMappingBlocked err: ", err)
		return HF_JT_BLOCKED_LABEL, "The new employee was hired with no Job Title so they will not be added to Active Directory.", createdCredentials
	}
	if adUsername == nil {
		ErrorLog.Println("AD was blocked by cost center")

		reason := "The new employee was hired without being given a Job, therefore there is no template to build an Active Directory User from."
		if jobTitle != nil {
			reason = fmt.Sprintf("The new employee was given the Job Title of %s, which is not assigned an Active Directory Template.", *jobTitle)
		}

		return HF_JT_BLOCKED_LABEL, reason, createdCredentials
	}

	InfoLog.Println("add AD user: ", newHiredEvent.Email, ", been processed, waiting for OPS")

	return HF_PENDING_LABEL, fmt.Sprintf("Received new hire, waiting for your On-Premise Service to add user to AD."), createdCredentials
}

func adFired(newFiredEvent *HFEvent, fullEmployeeRecord opapi.OPFullEmployeeRecord) (string, string) {
	thisCompanyIntegration, err := getCompanyIntegrationByIntegrationString(adIntegrationURL, newFiredEvent.CompanyID)
	if err != nil {
		ErrorLog.Println("adFired couldnt look up company integration: ", err)
		return HF_FAILED_LABEL, "An error occured on our end, we will investigate this issue."
	}

	newFiredEvent.Email = fullEmployeeRecord.Username
	*newFiredEvent.Processed = true
	dbmap.Update(newFiredEvent)

	if !checkIntegrationEnabled(thisCompanyIntegration) {
		InfoLog.Println("new fired but AD disabled")

		return HF_DISABLED_LABEL, "AD Integration was disabled at the time of processing event."
	}

	co, _ := lookupCompanyByID(thisCompanyIntegration.CompanyID)

	adUsername, jobTitle, err := checkHFTemplateMapping(co.OPID, thisCompanyIntegration, fullEmployeeRecord)
	if err != nil {
		ErrorLog.Println("adFired checkHFTemplateMappingBlocked err: ", err)
		return HF_JT_BLOCKED_LABEL, "The new employee was terminated with no Job Title so this action will not be sent to Active Directory."
	}
	if adUsername == nil {
		ErrorLog.Println("adFired was blocked by cost center")

		reason := "The terminated employee had not been given a Job, therefore this action will not be sent to Active Directory."
		if jobTitle != nil {
			reason = fmt.Sprintf("The terminated employee had the Job Title of %s, which is not assigned an Active Directory Template.", *jobTitle)
		}

		return HF_JT_BLOCKED_LABEL, reason
	}

	InfoLog.Println("fired AD user: ", fullEmployeeRecord.Username, ", been processed, waiting for OPS")

	return HF_PENDING_LABEL, fmt.Sprintf("Received termination of %s, waiting for your On-Premise Service to process and inactivate from AD.", fullEmployeeRecord.Username)
}
