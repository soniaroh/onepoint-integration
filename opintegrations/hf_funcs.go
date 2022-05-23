package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"opapi"
)

type NewHiresReport struct {
	OPCompanyID   int64  `csv:"Company: Id"`
	AccountID     int64  `csv:"Account Id"`
	FirstName     string `csv:"First Name"`
	LastName      string `csv:"Last Name"`
	DateHired     string `csv:"Date Hired"`
	DateReHired   string `csv:"Date Re-Hired"`
	DateStarted   string `csv:"Date Started"`
	ReportDate    string `csv:"Report Date"`
	Email         string `csv:"Email"`
	AccountStatus string `csv:"Employee Status"`
}

type NewFiresReport struct {
	OPCompanyID      int64  `csv:"Company: Id"`
	CompanyShortName string `csv:"Company: Short Name"`
	AccountID        int64  `csv:"Account Id"`
	FirstName        string `csv:"First Name"`
	LastName         string `csv:"Last Name"`
	AccountStatus    string `csv:"Account Status"`
	Locked           string `csv:"Locked"`
	Email            string `csv:"Email"`
	DateTerminated   string `csv:"Date Terminated"`
}

type HFCompanyIntegrationResultJoin struct {
	CompanyIntegration   CompanyIntegrationPublic
	Result               HFResult
	GeneratedCredentials HFCreatedCredentials
}

type HFCreatedCredentials struct {
	Username string
	Password string
}

// from jshahbazian ENADMIN
// const (
// 	HIRED_REPORT_ID = "37331748"
// 	FIRED_REPORT_ID = "37331753"
// )

// from APIUSER ENADMIN
const (
	HIRED_REPORT_ID = "37459660"
	FIRED_REPORT_ID = "37459661"
)

func hiredFiredPull() {
	getNewHires()
	getNewFires()
	processNewHiresAndFires()
}

func addHFResult(coIntID int64, status string, hfEventID int64, description string) (HFResult, error) {
	newHFResult := HFResult{
		CompanyIntegrationID: coIntID,
		Status:               status,
		HFEventID:            hfEventID,
		Description:          description,
		Created:              time.Now().Unix(),
	}

	err := dbmap.Insert(&newHFResult)

	return newHFResult, err
}

func processNewHiresAndFires() {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		ErrorLog.Println(err)
		return
	}

	// we process unprocessed events <= today
	todayStr := time.Now().In(loc).Format("01/02/2006")

	newHFEvents := []HFEvent{}
	_, err = dbmap.Select(&newHFEvents, `SELECT * FROM hf_events WHERE processed=0 AND STR_TO_DATE(event_date, '%m/%d/%Y') <= STR_TO_DATE(?, '%m/%d/%Y')`, todayStr)
	if err != nil {
		ErrorLog.Println("couldnt look up newHFEvents: ", err)
		return
	}

	for _, newHFEvent := range newHFEvents {
		InfoLog.Println("starting to process new event ID=", newHFEvent.ID)

		lookupHFCIsAndRun(&newHFEvent)
	}
}

func lookupHFCIsAndRun(newHFEvent *HFEvent) {
	product, err := lookupProductByURL(PRODUCT_USERPROVISIONING_URL)
	if err != nil {
		ErrorLog.Println("lookupHFCIsAndRun lookupProductByURL err: ", err)
		return
	}

	companyProduct, err := getCompanyProduct(newHFEvent.CompanyID, product.ID)
	if err != nil {
		ErrorLog.Println("getCompanyProduct err: ", err)
		return
	}

	allCIs, err := getCompaniesCIByProduct(product.ID, newHFEvent.CompanyID)
	if err != nil {
		ErrorLog.Println("lookupHFCIsAndRun getCompaniesCIByProduct err: ", err)
		return
	}

	opCompanyID, _ := getOPCompanyID(newHFEvent.CompanyID)
	cxn := chooseOPAPICxn(opCompanyID)
	fullEmployeeRecord, err := cxn.GetFullEmployeeInfo(newHFEvent.OPAccountID, opCompanyID)
	if err != nil {
		fullEmployeeRecord = opapi.OPFullEmployeeRecord{
			PrimaryEmail: newHFEvent.Email,
			FirstName:    newHFEvent.FirstName,
			LastName:     newHFEvent.LastName,
		}
	}

	// get email providers in front so they go first, bc others can depend on them
	sort.Sort(EmailProvidersInFront(allCIs))

	allResults := []HFCompanyIntegrationResultJoin{}
	for _, ci := range allCIs {
		var resultLabel, resultDesc string
		var credentials HFCreatedCredentials

		if newHFEvent.Type == HIRED_TYPE_ID {
			if fullEmployeeRecord.Dates.Terminated != "" {
				InfoLog.Println("Processing hiring but employee already terminated")
				InfoLog.Printf("Account id: %v, company id: %v \n", fullEmployeeRecord.ID, newHFEvent.CompanyID)
				resultLabel = HF_OVERRIDDEN
				resultDesc = "The employee has already been terminated. No accounts will be provisioned."
				*newHFEvent.Processed = true
				dbmap.Update(newHFEvent)
			}

			if resultLabel == "" {
				if ci.URL == gSuiteIntegrationsURL {
					resultLabel, resultDesc, credentials = gSuiteHired(newHFEvent, fullEmployeeRecord)
				} else if ci.URL == m365IntegrationsURL {
					resultLabel, resultDesc, credentials = m365Hired(newHFEvent, fullEmployeeRecord)
				} else if ci.URL == sfIntegrationsURL {
					resultLabel, resultDesc, credentials = sfHired(newHFEvent, opCompanyID, fullEmployeeRecord, allResults)
				} else if ci.URL == boxIntegrationsURL {
					resultLabel, resultDesc, credentials = boxHired(newHFEvent, opCompanyID, fullEmployeeRecord, allResults)
				} else if ci.URL == dropboxIntegrationsURL {
					resultLabel, resultDesc, credentials = dropboxHired(newHFEvent, opCompanyID, fullEmployeeRecord, allResults)
				} else if ci.URL == adIntegrationURL {
					resultLabel, resultDesc, credentials = adHired(newHFEvent, opCompanyID, fullEmployeeRecord, allResults)
				}
			}
		} else if newHFEvent.Type == FIRED_TYPE_ID {
			if checkTerminatedBeforeStarted(newHFEvent.CompanyID, fullEmployeeRecord.Dates.Terminated, fullEmployeeRecord.Dates.Hired, fullEmployeeRecord.Dates.Started, fullEmployeeRecord.Dates.ReHired) {
				InfoLog.Println("Processing firing but employee has date hired or date started in the future (they never started or got accounts provisioned)")
				InfoLog.Printf("Account id: %v, company id: %v \n", fullEmployeeRecord.ID, newHFEvent.CompanyID)
				resultLabel = HF_OVERRIDDEN
				resultDesc = "The employee was terminated before their accounts were provisioned. No action will be taken."
				*newHFEvent.Processed = true
				dbmap.Update(newHFEvent)
			}

			if resultLabel == "" {
				if ci.URL == gSuiteIntegrationsURL {
					resultLabel, resultDesc = gSuiteFired(newHFEvent, fullEmployeeRecord)
				} else if ci.URL == m365IntegrationsURL {
					resultLabel, resultDesc = m365Fired(newHFEvent, fullEmployeeRecord)
				} else if ci.URL == sfIntegrationsURL {
					resultLabel, resultDesc = sfFired(newHFEvent, fullEmployeeRecord)
				} else if ci.URL == boxIntegrationsURL {
					resultLabel, resultDesc = boxFired(newHFEvent, fullEmployeeRecord)
				} else if ci.URL == dropboxIntegrationsURL {
					resultLabel, resultDesc = dropboxFired(newHFEvent, fullEmployeeRecord)
				} else if ci.URL == adIntegrationURL {
					resultLabel, resultDesc = adFired(newHFEvent, fullEmployeeRecord)
				}
			}
		}

		newHFResult, err := addHFResult(ci.ID, resultLabel, newHFEvent.ID, resultDesc)
		if err != nil {
			ErrorLog.Printf("err creating HF Result: %v", err)
		}

		if newHFResult.Status != HF_SUCCESS_LABEL && newHFResult.Status != HF_FAILED_LABEL {
			// ignore disabled AND pending
			continue
		}

		resultJoin := HFCompanyIntegrationResultJoin{
			CompanyIntegration:   ci,
			Result:               newHFResult,
			GeneratedCredentials: credentials,
		}

		allResults = append(allResults, resultJoin)
	}

	if len(allResults) > 0 {
		sendHFSummaryEmail(companyProduct.Settings, fullEmployeeRecord, newHFEvent, allResults)
	}
}

func checkTerminatedBeforeStarted(companyID int64, dateTerminated, dateHired, dateStarted, dateRehired string) bool {
	dateTerminatedT, _ := time.Parse("2006-01-02", dateTerminated)

	if dateRehired != "" {
		dateRehiredT, _ := time.Parse("2006-01-02", dateRehired)
		if dateTerminatedT.Unix() <= dateRehiredT.Unix() {
			return true
		}
	} else {
		dateHiredT, _ := time.Parse("2006-01-02", dateHired)
		dateStartedT, _ := time.Parse("2006-01-02", dateStarted)

		processDate := dateHiredT

		coProd, _ := getCompanyProductByURL(companyID, PRODUCT_USERPROVISIONING_URL)
		settingValue, err := getHiredEventTimingSettingFromCompanyProduct(coProd)
		if err != nil {
			ErrorLog.Println("checkTerminatedBeforeStarted err: " + err.Error())
		} else {
			switch settingValue {
			case "date_hired":
				// already is dateHiredT
			case "date_started_sub_2":
				processDate = dateStartedT.AddDate(0, 0, -2)
			case "date_started_sub_1":
				processDate = dateStartedT.AddDate(0, 0, -1)
			case "date_started":
				processDate = dateStartedT
			}
		}

		if dateTerminatedT.Unix() <= processDate.Unix() {
			return true
		}
	}

	return false
}

func getNewHires() {
	product, err := lookupProductByURL(PRODUCT_USERPROVISIONING_URL)
	if err != nil {
		ErrorLog.Println("lookupProductByURL err: ", err)
		return
	}

	companyMap := getCompaniesMap(product.ID)
	if companyMap == nil {
		ErrorLog.Println("getCompaniesMap err, quitting")
		return
	}

	InfoLog.Println("starting new HIRED pull")

	hireRows := []NewHiresReport{}
	reportURL := fmt.Sprintf("https://secure.onehcm.com/ta/rest/v1/report/%s/%v", "saved", HIRED_REPORT_ID)
	err = hfReportsCxn.GenricReport(reportURL, &hireRows)
	if err != nil {
		ErrorLog.Println("err! ", err)
		return
	}

	for _, hireRow := range hireRows {
		_, isCompanyWeWant := companyMap[hireRow.OPCompanyID]
		if isCompanyWeWant {

			dateToProcess := hireRow.ReportDate
			if hireRow.DateReHired == "" { // if this is a rehire, must use rehire date
				dateHired, _ := time.Parse("01/02/2006", hireRow.ReportDate)
				dateStarted, _ := time.Parse("01/02/2006", hireRow.DateStarted)

				if dateHired.Unix() <= dateStarted.Unix() { // if this is not true, just use date hired
					coProd, _ := getCompanyProductByURL(companyMap[hireRow.OPCompanyID], PRODUCT_USERPROVISIONING_URL)
					settingValue, err := getHiredEventTimingSettingFromCompanyProduct(coProd)
					if err != nil {
						ErrorLog.Println("getHiredEventTimingSettingFromCompanyProduct err: " + err.Error())
					} else {
						switch settingValue {
						case "date_hired":
							// already is hireRow.ReportDate (date hired)
						case "date_started_sub_2":
							daysToSub := -2

							if dateStarted.Weekday() == time.Monday || dateStarted.Weekday() == time.Tuesday {
								daysToSub = -4
							}

							newDate := dateStarted.AddDate(0, 0, daysToSub)
							dateToProcess = newDate.Format("01/02/2006")
						case "date_started_sub_1":
							daysToSub := -1

							if dateStarted.Weekday() == time.Monday {
								daysToSub = -3
							}

							newDate := dateStarted.AddDate(0, 0, daysToSub)
							dateToProcess = newDate.Format("01/02/2006")
						case "date_started":
							dateToProcess = hireRow.DateStarted
						}
					}
				}
			}

			existingHFEvents := []HFEvent{}
			_, err := dbmap.Select(&existingHFEvents, "SELECT * FROM hf_events WHERE op_account_id = ?", hireRow.AccountID)
			if err != nil {
				ErrorLog.Println("-- couldnt lookup existing events: ", err)
				continue
			}

			skip := false

			sort.Sort(ByDateDesc(existingHFEvents))

			haveSeenFired := false
			for _, existingHFEvent := range existingHFEvents {
				if existingHFEvent.Type == FIRED_TYPE_ID && *existingHFEvent.Processed {
					haveSeenFired = true
					continue
				}

				if existingHFEvent.Type == HIRED_TYPE_ID {
					if existingHFEvent.EventDate == dateToProcess {
						// same event and date, we ignore
						skip = true
						break
					} else if !*existingHFEvent.Processed {
						// theres an existing nonprocessed hire event for this emp but
						// different day, most likely it has been overriden by this one in OP
						err = deleteHFEvent(&existingHFEvent)
						if err != nil {
							ErrorLog.Printf("tried to delete HF EVENT %+v but failed: %+v\n", existingHFEvent, err)
							skip = true
						}
					} else if *existingHFEvent.Processed {
						// this is the last processed hiring
						if !haveSeenFired {
							// we are seeing a hiring and we have not seen a firing yet,
							// so we should not hire again, this could indicate a changed timing setting
							skip = true
							break
						}
					}
				}
			}

			if skip {
				continue
			}

			processed := false

			newHFEvent := HFEvent{
				Type:        HIRED_TYPE_ID,
				OPAccountID: hireRow.AccountID,
				EventDate:   dateToProcess,
				CompanyID:   companyMap[hireRow.OPCompanyID],
				FirstName:   hireRow.FirstName,
				LastName:    hireRow.LastName,
				Email:       hireRow.Email,
				Processed:   &processed,
				Created:     time.Now().Unix(),
			}

			err = dbmap.Insert(&newHFEvent)
			if err != nil {
				ErrorLog.Println("-- couldnt Insert new event: ", err)
			} else {
				InfoLog.Println("-- added new hire: acct_id: ", hireRow.AccountID, ", - report date: ", dateToProcess)
			}
		}
	}
}

func getNewFires() {
	product, err := lookupProductByURL(PRODUCT_USERPROVISIONING_URL)
	if err != nil {
		ErrorLog.Println("lookupProductByURL err: ", err)
		return
	}

	companyMap := getCompaniesMap(product.ID)
	if companyMap == nil {
		ErrorLog.Println("getCompaniesMap err, quitting")
		return
	}

	InfoLog.Println("starting new FIRED pull")

	fireRows := []NewFiresReport{}
	reportURL := fmt.Sprintf("https://secure.onehcm.com/ta/rest/v1/report/%s/%v", "saved", FIRED_REPORT_ID)
	err = hfReportsCxn.GenricReport(reportURL, &fireRows)
	if err != nil {
		ErrorLog.Println("err! ", err)
		return
	}

	for _, fireRow := range fireRows {
		_, isCompanyWeWant := companyMap[fireRow.OPCompanyID]
		if isCompanyWeWant {
			existingHFEvents := []HFEvent{}
			_, err := dbmap.Select(&existingHFEvents, "SELECT * FROM hf_events WHERE op_account_id = ?", fireRow.AccountID)
			if err != nil {
				ErrorLog.Println("-- couldnt lookup existing events: ", err)
				continue
			}

			skip := false
			for _, existingHFEvent := range existingHFEvents {
				if existingHFEvent.Type == FIRED_TYPE_ID {
					if existingHFEvent.EventDate == fireRow.DateTerminated {
						// same event and date, we ignore
						skip = true
						break
					} else if !*existingHFEvent.Processed {
						// theres an existing nonprocessed fire event for this emp but
						// different day, most likely it has been overriden by this one in OP
						err = deleteHFEvent(&existingHFEvent)
						if err != nil {
							ErrorLog.Printf("tried to delete HF EVENT %+v but failed: %+v\n", existingHFEvent, err)
							skip = true
						}
					}
				}
			}

			if skip {
				continue
			}

			processed := false

			newHFEvent := HFEvent{
				Type:        FIRED_TYPE_ID,
				OPAccountID: fireRow.AccountID,
				EventDate:   fireRow.DateTerminated,
				CompanyID:   companyMap[fireRow.OPCompanyID],
				FirstName:   fireRow.FirstName,
				LastName:    fireRow.LastName,
				Email:       fireRow.Email,
				Processed:   &processed,
				Created:     time.Now().Unix(),
			}

			err = dbmap.Insert(&newHFEvent)
			if err != nil {
				ErrorLog.Println("-- couldnt Insert new event: ", err)
			} else {
				InfoLog.Println("-- added new fire: acct_id: ", fireRow.AccountID, ", - termination date: ", fireRow.DateTerminated)
			}
		}
	}
}

func deleteHFEvent(hfEvent *HFEvent) (err error) {
	_, err = dbmap.Delete(hfEvent)
	return err
}

func checkIntegrationEnabled(coInt CompanyIntegration) bool {
	enabled, ok := coInt.Settings["enabled"].(bool)
	if ok {
		return enabled
	}

	ErrorLog.Println("checkIntegrationEnabled coInt.Settings[enabled] was not bool")
	return false
}

func checkHFCostCenterBlocked(coInt CompanyIntegration, fullEmployeeRecord opapi.OPFullEmployeeRecord) (bool, error) {
	blocked := false

	costCenterFilterSetting, ok := coInt.Settings["cost_center_filter"].(map[string]interface{})
	if !ok {
		return blocked, errors.New("Could not typecast cost_center_filter")
	}

	ccAllEmployeesBool, ok := costCenterFilterSetting["all_employees"].(bool)
	if !ok {
		return blocked, errors.New("Could not typecast cost_center_filter.all_employees")
	}

	if ccAllEmployeesBool {
		return false, nil
	}

	ccsInterfaces, ok := costCenterFilterSetting["ccs"].([]interface{})
	if !ok {
		return blocked, errors.New("Could not typecast cost_center_filter.ccs")
	}

	thisEmployeesCostCenters := make(map[int]bool)
	for _, cc := range fullEmployeeRecord.CostCentersInfo.Defaults {
		thisEmployeesCostCenters[cc.Value.ID] = true
	}

	foundOne := false
	for i, costCenterInterface := range ccsInterfaces {
		costCenter, ok := costCenterInterface.(map[string]interface{})
		if !ok {
			return blocked, errors.New(fmt.Sprintf("Could not typecast cost_center_filter.ccs[%d]", i))
		}

		ccID, ok := costCenter["id"].(float64)
		if !ok {
			return blocked, errors.New(fmt.Sprintf("Could not typecast cost_center_filter.ccs[%d].id", i))
		}

		_, existsInEmployeeRecord := thisEmployeesCostCenters[int(ccID)]
		if existsInEmployeeRecord {
			foundOne = true
			break
		}
	}

	if foundOne {
		return false, nil
	}

	return true, nil
}

func generateEmailAddressBasedOnSettings(fullEmployeeRecord opapi.OPFullEmployeeRecord, coInt CompanyIntegration, resultsSoFar []HFCompanyIntegrationResultJoin) (string, error) {
	emailToUse := fullEmployeeRecord.PrimaryEmail

	emailSettingMap, ok := coInt.Settings["email_name"].(map[string]interface{})
	if ok {
		choiceStr, ok := emailSettingMap["choice"].(string)
		domainName, domainOK := emailSettingMap["domain_name"].(string)
		if ok {
			switch choiceStr {
			case EMAIL_PATTERN_FROM_OP:
				if fullEmployeeRecord.PrimaryEmail == "" {
					return "", errors.New("No Primary Email at time of hire.")
				}
			case EMAIL_PATTERN_OP_USERNAME:
				if domainOK {
					emailToUse = fmt.Sprintf("%s@%s", fullEmployeeRecord.Username, domainName)
				} else {
					ErrorLog.Println("generateEmailAddressBasedOnSettings choice = EMAIL_PATTERN_OP_USERNAME but domain not OK")
				}
			case EMAIL_PATTERN_ADJACENT:
				if domainOK {
					emailToUse = fmt.Sprintf("%s%s@%s", strings.ToLower(fullEmployeeRecord.FirstName), strings.ToLower(fullEmployeeRecord.LastName), domainName)
				} else {
					ErrorLog.Println("generateEmailAddressBasedOnSettings choice = EMAIL_PATTERN_ADJACENT but domain not OK")
				}
			case EMAIL_PATTERN_PERIOD:
				if domainOK {
					emailToUse = fmt.Sprintf("%s.%s@%s", strings.ToLower(fullEmployeeRecord.FirstName), strings.ToLower(fullEmployeeRecord.LastName), domainName)
				} else {
					ErrorLog.Println("generateEmailAddressBasedOnSettings choice = EMAIL_PATTERN_ADJACENT but domain not OK")
				}
			case EMAIL_PATTERN_FIRST_INITIAL:
				if domainOK {
					emailToUse = fmt.Sprintf("%s%s@%s", strings.ToLower(string(fullEmployeeRecord.FirstName[0])), strings.ToLower(fullEmployeeRecord.LastName), domainName)
				} else {
					ErrorLog.Println("generateEmailAddressBasedOnSettings choice = EMAIL_PATTERN_ADJACENT but domain not OK")
				}
			case gSuiteIntegrationsURL, m365IntegrationsURL:
				var err error
				emailToUse, err = findPreviouslyGeneratedEmail(choiceStr, resultsSoFar)
				if err != nil {
					return "", err
				}
			default:
				ErrorLog.Println("generateEmailAddressBasedOnSettings choiceStr did not match any choices")
				return "", errors.New("No email setting found.")
			}
		} else {
			ErrorLog.Println("generateEmailAddressBasedOnSettings but choice was not string")
		}
	} else {
		ErrorLog.Println("generateEmailAddressBasedOnSettings but setting was not map[string]interface{}")
	}

	return emailToUse, nil
}

func findPreviouslyGeneratedEmail(intURL string, resultsSoFar []HFCompanyIntegrationResultJoin) (string, error) {
	for _, prevResultStruct := range resultsSoFar {
		if prevResultStruct.CompanyIntegration.URL == intURL {
			if prevResultStruct.Result.Status == HF_SUCCESS_LABEL {
				InfoLog.Printf("successfully found previously generated email address")
				return prevResultStruct.GeneratedCredentials.Username, nil
			} else {
				InfoLog.Printf("findPreviouslyGeneratedEmail but it failed: url: %s", intURL)
				return "", errors.New("The reference email integration failed to create an email address.")
			}
		}
	}

	ErrorLog.Printf("findPreviouslyGeneratedEmail but didnt return from loop, resultsSoFar: %v\n", resultsSoFar)
	return "", errors.New("The reference email integration does not exist")
}

func sendNewEmailToOP(companyID int64, employeeRecord opapi.OPFullEmployeeRecord, newEmailAddress string) error {
	if employeeRecord.PrimaryEmail == newEmailAddress {
		InfoLog.Println("sendNewEmailToOP but emails are same")
		return errors.New("OnePoint Primary Email not updated.")
	}

	newPrimary, newSecondary := newEmailAddress, ""
	if employeeRecord.PrimaryEmail != "" {
		newSecondary = employeeRecord.PrimaryEmail
	}

	opID, err := getOPCompanyID(companyID)
	if err != nil {
		ErrorLog.Println("Cannot convert internal co ID to OP co ID: ", err)
		return errors.New("OnePoint Primary Email not updated.")
	}

	url := fmt.Sprintf("https://secure.onehcm.com/ta/rest/v2/companies/%d/employees/%d", opID, employeeRecord.ID)

	empMap := make(map[string]interface{})
	cxn := chooseOPAPICxn(opID)
	err = cxn.BasicQuery(url, &empMap, false)
	if err != nil {
		ErrorLog.Println("opQueryAPI failed in sendNewEmailToOP: ", err)
		return errors.New("OnePoint Primary Email not updated.")
	}

	empMap["primary_email"] = newPrimary
	empMap["secondary_email"] = newSecondary

	err = cxn.OPGenericRequest(url, "PUT", empMap)
	if err != nil {
		ErrorLog.Println("opGenericRequest failed in sendNewEmailToOP: ", err)
		return errors.New("OnePoint Primary Email not updated.")
	}

	return nil
}

func checkHFConnectionStatuses() {
	integration, _ := lookupIntegrationByURL(adIntegrationURL)

	allADCIs, err := getCompanyIntegrationsUsingIntegrationURL(integration.URL)
	if err != nil {
		ErrorLog.Println("checkHFConnectionStatuses cannot lookup CIs: " + err.Error())
		return
	}

	MINUTES_AGO := -21
	timeToBeAfterSecs := time.Now().Add(time.Duration(MINUTES_AGO) * time.Minute).Unix()

	for _, adCI := range allADCIs {
		if adCI.Status != "connected" {
			continue
		}

		lastPingedSecs, ok := adCI.Settings[HF_LAST_PING_SETTING].(float64)
		if !ok {
			ErrorLog.Printf("active directory last ping setting CI: %d, cannt type cast setting\n", adCI.ID)
			continue
		}

		if int64(lastPingedSecs) < timeToBeAfterSecs {
			adCI.Status = "disconnected"
			_, err = dbmap.Update(&adCI)
			if err != nil {
				ErrorLog.Printf("checkHFConnectionStatuses cannot update CI: %+v\n", adCI)
				continue
			}

			if checkIntegrationEnabled(adCI) {
				go sendActiveDirectoryDisconnectedEmail(integration.URL, integration.Name, adCI)
			}

			InfoLog.Printf("ACTIVE DIRECTORY HAS BECOME DISCONNECTED, co: %d\n", adCI.CompanyID)
		}
	}
}

func testCheckHFTemplateMapping() {
	cxn := chooseOPAPICxn(33560858)
	record, _ := cxn.GetFullEmployeeInfo(48614248, 33560858)
	ci, _ := getCompanyIntegrationByIntegrationString("activedirectory", 1)
	un, jt, err := checkHFTemplateMapping(33560858, ci, record)
	if jt != nil {
		if un != nil {
			InfoLog.Println(*un, *jt, err)
		} else {
			InfoLog.Println(un, *jt, err)
		}
	} else {
		InfoLog.Println(un, jt, err)
	}
}

func checkHFTemplateMapping(opCompanyID int64, coInt CompanyIntegration, fullEmployeeRecord opapi.OPFullEmployeeRecord) (adUsername, jobTitle *string, err error) {
	cxn := chooseOPAPICxn(opCompanyID)

	payInfo, err := cxn.GetEmployeePayInfo(int64(fullEmployeeRecord.ID), opCompanyID)
	if err != nil {
		ErrorLog.Println("GetEmployeePayInfo err: " + err.Error())
		return adUsername, jobTitle, err
	}

	jobs, err := cxn.GetOPJobs(opCompanyID)
	if err != nil {
		ErrorLog.Println("checkHFTemplateMapping err: " + err.Error())
		return adUsername, jobTitle, err
	}

	var thisEmpJob *opapi.OPJob
	for _, job := range jobs {
		if job.ID == payInfo.DefaultJob.ID {
			thisEmpJob = &job
		}

		if thisEmpJob != nil {
			break
		}
	}

	if thisEmpJob == nil {
		ErrorLog.Println(fmt.Sprintf("could not find employees job! employee account id: %d, company op id: %d, job id: %d", fullEmployeeRecord.ID, opCompanyID, payInfo.DefaultJob.ID))
		return adUsername, jobTitle, errors.New("jobnotfound")
	}

	jobTitle = &thisEmpJob.Name

	setting, ok := coInt.Settings["new_user_templates"].(map[string]interface{})
	if !ok {
		ErrorLog.Println("Could not typecast new_user_templates")
		return adUsername, jobTitle, errors.New("Could not typecast new_user_templates")
	}

	templatesIntfcs, ok := setting["templates"].([]interface{})
	if !ok {
		ErrorLog.Println("Could not typecast new_user_templates.templates")
		return adUsername, jobTitle, errors.New("Could not typecast new_user_templates.templates")
	}

	for _, templateInt := range templatesIntfcs {
		template, ok := templateInt.(map[string]interface{})
		if !ok {
			ErrorLog.Println("could not typecast templateInt")
			continue
		}

		username, ok := template["ad_username"].(string)
		if !ok {
			ErrorLog.Println("could not typecast ad_username")
			continue
		}

		valuesInts, ok := template["op_values"].([]interface{})
		if !ok {
			ErrorLog.Println("could not typecast values")
			continue
		}

		for _, valueInt := range valuesInts {
			value, ok := valueInt.(map[string]interface{})
			if !ok {
				ErrorLog.Println("could not typecast valueInt")
				continue
			}

			valueID, ok := value["id"].(float64)
			if !ok {
				ErrorLog.Println("could not typecast id")
				continue
			}

			if valueID == float64(thisEmpJob.ID) {
				InfoLog.Printf("FOUND THE JOB!\n")
				adUsername = &username
				break
			}
		}

		if adUsername != nil {
			break
		}
	}

	return
}

func getNewPasswordFromSetting(ci CompanyIntegration) (string, error) {
	newPWInterface, ok := ci.Settings["new_password"].(map[string]interface{})
	if !ok {
		return "", errors.New("new_password setting not found")
	}

	valueStr, ok := newPWInterface["value"].(string)
	if !ok {
		return "", errors.New("new_password setting has no value")
	}

	return valueStr, nil
}

func getHiredEventTimingSettingFromCompanyProduct(cp CompanyProduct) (string, error) {
	timingSettingInterface, ok := cp.Settings["hired_timing"].(map[string]interface{})
	if !ok {
		return "", errors.New("hired_timing setting not found")
	}

	valueStr, ok := timingSettingInterface["choice"].(string)
	if !ok {
		return "", errors.New("hired_timing setting has no value")
	}

	return valueStr, nil
}
