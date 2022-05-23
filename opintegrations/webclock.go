package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"sort"
	"strings"
	"time"
	"unsafe"

	"opapi"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

type WebClockRequest struct {
	CompanyShortname string  `json:"shortname"`
	Username         string  `json:"username"`
	Password         string  `json:"password"`
	PunchType        string  `json:"type"`
	Photo            *string `json:"photo"`

	Timestamp  string `json:"timestamp"`
	EmployeeID string `json:"employee_id"`

	CostCenters *[]WebClockCCArg `json:"cost_centers"`

	IsFinalOut *bool `json:"is_final_out"`
}

type WebClockNameRequest struct {
	CompanyShortname string `json:"shortname"`
	Username         string `json:"username"`
	Password         string `json:"password"`
	EmployeeID       string `json:"employee_id"`
}

type WebClockCCArg struct {
	Index int              `json:"index"`
	Value opapi.CostCenter `json:"value"`
}

type WebClockAttestationRequest struct {
	Company  *Company                    `json:"company"`
	Answers  []WebClockAttestationAnswer `json:"answers"`
	TimeData *opapi.EmployeeTimeEntries  `json:"time_data"`
}

type ByEndTimeInc []opapi.TimeEntry

const timestampFormat = "2006-01-02T15:04:05.000-07:00"

func (a ByEndTimeInc) Len() int { return len(a) }
func (a ByEndTimeInc) Less(i, j int) bool {
	if a[i].CalcEndTime == nil {
		return true
	} else {
		if a[j].CalcEndTime == nil {
			return false
		}
	}
	iTime, err := time.Parse(timestampFormat, *a[i].CalcEndTime)
	if err != nil {
		return true
	}
	jTime, err := time.Parse(timestampFormat, *a[j].CalcEndTime)
	if err != nil {
		return false
	}

	return iTime.Unix() < jTime.Unix()
}
func (a ByEndTimeInc) Swap(i, j int) { a[i], a[j] = a[j], a[i] }

type WebClockAttestationAnswer struct {
	StepID               string  `json:"step_id"`
	Value                string  `json:"value"`
	Explanation          *string `json:"explanation"`
	SavedValue           bool    `json:"saved_value"`
	SavedExtraFieldIndex int     `json:"saved_extra_field_index"`
}

type WebClockPunch struct {
	ID             int64  `db:"id, primarykey, autoincrement"`
	CompanyID      int64  `db:"company_id" json:"company_id"`
	AccountID      int64  `db:"account_id" json:"account_id"`
	PunchType      string `db:"punch_type" json:"punch_type"`
	PhotoLink      string `db:"photo_link" json:"photo_link"`
	PunchTimestamp string `db:"punch_timestamp" json:"punch_timestamp"`
}

type WebClockSettings struct {
	IPSetting struct {
		AllowAll   bool     `json:"allow_all"`
		AllowedIPs []string `json:"allowed_ips"`
	} `json:"ip"`
	InputsSetting struct {
		Username   bool `json:"username"`
		Password   bool `json:"password"`
		Photo      bool `json:"photo"`
		EmployeeID bool `json:"employee_id"`
	} `json:"inputs"`
	ActionsSetting struct {
		CCChange WebClockCCActionSetting `json:"cc_change"`
	} `json:"actions"`
	Enabled              bool                       `json:"enabled"`
	Attestation          WebClockAttestationSetting `json:"attestation"`
	ConfirmIdentityFirst struct {
		Active bool `json:"active"`
	} `json:"identify_first"`
	LocationByIP struct {
		Active   bool `json:"active"`
		Required bool `json:"required"`
	} `json:"location_by_ip"`
}

type WebClockCCActionSetting struct {
	Active    bool   `json:"active"`
	UseLimits bool   `json:"use_employee_limits"`
	Label     string `json:"label"`
}

type WebClockAttestationSetting struct {
	Active           bool                               `json:"active"`
	SpanishAvailable bool                               `json:"spanish_available"`
	FirstStepID      *string                            `json:"first_step_id"`
	LastStepID       *int                               `json:"last_step_id"`
	StepIDs          []string                           `json:"step_ids"`
	Steps            map[string]WebClockAttestationStep `json:"steps"`
}

type WebClockAttestationStep struct {
	Type                 string `json:"type"`
	SavedValue           *bool  `json:"saved_value"`
	SavedExtraFieldIndex *int   `json:"saved_extra_field_index"`
	InferredCondition    struct {
		NotTake30     *bool   `json:"not_take_30"`
		TotalHoursGte *string `json:"total_hours_gte"`
		TotalHoursLte *string `json:"total_hours_lte"`
	} `json:"inferred_condition"`
	Outcomes []struct {
		InferredResult *bool   `json:"inferred_result"`
		NextStep       *string `json:"next_step"`
		Value          *string `json:"value"`
	} `json:"outcomes"`
}

const (
	PRODUCT_WEBCLOCK_URL    = "webclock"
	PUNCH_PHOTOS_BUCKETNAME = "punch-photos"
	PUNCH_PHOTOS_URL        = "https://storage.googleapis.com/punch-photos/"
)

func webClockGetHandler(c *gin.Context) {
	shortname, _ := c.GetQuery("shortname")

	co, err := lookupCompanyByShortname(shortname)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Company not found"})
		return
	}

	coProd, err := getCompanyProductByURL(co.ID, PRODUCT_WEBCLOCK_URL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Company not found"})
		return
	}

	if coProd.Enabled != nil && !*coProd.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Company not found"})
		return
	}

	cxn := chooseOPAPICxn(co.OPID)
	ccs, err := getAllCostCentersWithCache(cxn, &co, false)
	if err != nil {
		ErrorLog.Println("err looking up companyID: ", co.OPID, " GetAllCompanyCostCenters: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Company not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"company": co, "settings": coProd.Settings, "costCenters": ccs})
}

func webClockAttestationPostHandler(c *gin.Context) {
	logPrefix := fmt.Sprintf("webClock ATT ")

	input := &WebClockAttestationRequest{}
	if err := c.ShouldBindWith(input, binding.JSON); err != nil {
		ErrorLog.Println("err binding webClockHandler json: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	if input.Company == nil || input.TimeData == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	empWjustID := &opapi.OPFullEmployeeRecord{ID: input.TimeData.Employee.AccountID}
	// replaced input.TimeData w timeDataTemp because need fresh data in case of photo in meantime.
	// problem with using the time data from the input is that a photo might have been uploaded
	// in the time between punch and submitting attestation data.
	// workaround is to check if there was a photo going to be uploaded,
	// can send an indicator (had_photo: true) in the punch response and relay it back here
	timeDataTemp, err := getEmployeeTimeEntries(input.Company, nil, empWjustID, 1, true)
	if err != nil {
		ErrorLog.Println("err getEmployeeTimeEntries: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "An error occured"})
		return
	}

	sort.Sort(ByEndTimeInc(timeDataTemp.TimeEntries))

	timeEntriesToSend := []opapi.TimeEntry{}

	addedAnswersToEntry := false
	var timeEntryID *int
	for i := len(timeDataTemp.TimeEntries) - 1; i >= 0; i-- {

		teReadOnly := timeDataTemp.TimeEntries[i]

		if (teReadOnly.StartTime == nil || *teReadOnly.StartTime == "") && (teReadOnly.EndTime == nil || *teReadOnly.EndTime == "") && (teReadOnly.Total == nil) {
			// exclude because the update will reject these
			fmt.Println("2")

			continue
		}

		if !addedAnswersToEntry {
			newCustomFields := []opapi.TimeEntryCustomFields{}

			indexesWeAreUpdating := make(map[int]bool)
			for _, answer := range input.Answers {
				if !answer.SavedValue {
					continue
				}
				indexesWeAreUpdating[answer.SavedExtraFieldIndex] = true

				newCustomFields = append(newCustomFields, opapi.TimeEntryCustomFields{Index: answer.SavedExtraFieldIndex, Value: answer.Value})
			}

			// add all existing cfs that are different indexes than our answers
			if teReadOnly.CustomFields != nil {
				for _, cusField := range *teReadOnly.CustomFields {
					if !indexesWeAreUpdating[cusField.Index] {
						newCustomFields = append(newCustomFields, cusField)
					}
				}
			}

			timeEntryID = teReadOnly.ID
			timeDataTemp.TimeEntries[i].CustomFields = &newCustomFields
			addedAnswersToEntry = true
		}

		// make fields nil that UPDATE does not accept but the GET sends
		timeDataTemp.TimeEntries[i].ID = nil
		timeDataTemp.TimeEntries[i].CalcPayCategory = nil
		timeDataTemp.TimeEntries[i].CalcStartTime = nil
		timeDataTemp.TimeEntries[i].CalcEndTime = nil
		timeDataTemp.TimeEntries[i].CalcTotal = nil
		timeDataTemp.TimeEntries[i].IsRaw = nil
		timeDataTemp.TimeEntries[i].IsCalc = nil

		timeEntriesToSend = append(timeEntriesToSend, timeDataTemp.TimeEntries[i])
	}

	timeDataTemp.TimeEntries = timeEntriesToSend
	updateBody := opapi.TimeEntriesUpdateRequest{TimeEntrySets: []opapi.EmployeeTimeEntries{*timeDataTemp}}

	cxn := chooseOPAPICxn(input.Company.OPID)
	_, opListResponse, err, opError := cxn.UpdateTimeEntries(input.Company.OPID, &updateBody)
	if err != nil {
		if opError != nil {
			ErrorLog.Printf(logPrefix, "opError: %+v\n", opError.Errors)
			c.JSON(http.StatusBadRequest, gin.H{"error": "An error occured"})
			return
		} else {
			ErrorLog.Printf(logPrefix, "err: %s\n", err.Error())
			c.JSON(http.StatusBadRequest, gin.H{"error": "An error occured"})
			return
		}
	}
	if len(opListResponse.Items) > 0 {
		if opListResponse.Items[0].Status == "FAILURE" {
			ErrorLog.Printf("%s opListResponse: %+v\n", logPrefix, opListResponse)
			c.JSON(http.StatusBadRequest, gin.H{"error": "An error occured"})
			return
		}
	} else {
		ErrorLog.Println(logPrefix, "len(opListResponse.Items) was 0")
		c.JSON(http.StatusBadRequest, gin.H{"error": "An error occured"})
		return
	}

	if timeEntryID != nil {
		// notesToAdd := []opapi.TimeEntryNote{}
		// for _, ans := range input.Answers {
		// 	if ans.Explanation != nil {
		// 		notesToAdd = append(notesToAdd, opapi.TimeEntryNote{TimeEntryID: *timeEntryID, Text: *ans.Explanation})
		// 	}
		// }
		// if len(notesToAdd) > 0 {
		// 	_, _, err, _ = cxn.CreateTimeEntryNote(notesToAdd)
		// 	if err != nil {
		// 		ErrorLog.Println(err)
		// 	}
		// }
	}

	c.JSON(http.StatusOK, nil)
}

// Must either send { username, password } OR { employee_id }
func webClockNamePostHandler(c *gin.Context) {
	input := &WebClockNameRequest{}
	if err := c.ShouldBindWith(input, binding.JSON); err != nil {
		ErrorLog.Println("err binding webClockHandler json: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	logPrefix := fmt.Sprintf("webClock GETNAME [%s %s %s] ", input.CompanyShortname, input.Username, input.EmployeeID)

	co, err := lookupCompanyByShortname(input.CompanyShortname)
	if err != nil {
		ErrorLog.Println(logPrefix, "lookupCompanyByShortname err: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Company not found"})
		return
	}

	var webclockSetting *WebClockSettings
	coProd, err := getCompanyProductByURL(co.ID, "webclock")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Company not found"})
		return
	}

	if coProd.Enabled != nil && !*coProd.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Company not found"})
		return
	}

	webclockSetting = coProd.Settings.ToWebclockSetting()
	if !webclockSetting.IPSetting.AllowAll {
		clientIP := c.GetHeader("X-Real-IP")
		if !env.Production {
			clientIP = "0.0.0.0"
		}

		allowed := false
		for _, ip := range webclockSetting.IPSetting.AllowedIPs {
			if clientIP == ip {
				allowed = true
				break
			}
		}

		if !allowed {
			ErrorLog.Println(logPrefix, "tried punch from disallowed IP ", clientIP)
			statement := "Your company prohibits punches from your current public IP Address of " + clientIP
			c.JSON(http.StatusBadRequest, gin.H{"error": statement})
			return
		}
	}

	var employee *opapi.OPFullEmployeeRecord
	if input.Username != "" && input.Password != "" {
		opCreds := opapi.OPCredentials{
			Username:         input.Username,
			Password:         input.Password,
			CompanyShortname: co.ShortName,
			APIKey:           co.APIKey,
		}

		opCxn := &opapi.OPConnection{
			Credentials: &opCreds,
		}

		err = opCxn.LoginAndSetToken()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		employee, err = findEmployeeByUsernameOrEmployeeID(&input.Username, nil, &co)
	} else if input.EmployeeID != "" {
		employee, err = findEmployeeByUsernameOrEmployeeID(nil, &input.EmployeeID, &co)
	}
	if err != nil {
		ErrorLog.Println(logPrefix, "findEmployeeByUsernameOrEmployeeID err: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Employee not found"})
		return
	}
	if employee == nil {
		ErrorLog.Println(logPrefix, "findEmployeeByUsernameOrEmployeeID err: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Employee not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"employee_name": employee.FirstName + " " + employee.LastName})
}

// Must either send { username, password } OR { employee_id }
func webClockPostHandler(c *gin.Context) {
	input := &WebClockRequest{}
	if err := c.ShouldBindWith(input, binding.JSON); err != nil {
		ErrorLog.Println("err binding webClockHandler json: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	logPrefix := fmt.Sprintf("webClock [%s %s %s %s] ", input.CompanyShortname, input.Username, input.EmployeeID, input.PunchType)

	switch input.PunchType {
	case "punch", "punch_in", "punch_out", "change_ccs":
		break
	default:
		ErrorLog.Println(logPrefix, "err wrong punch type: ", input.PunchType)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	co, err := lookupCompanyByShortname(input.CompanyShortname)
	if err != nil {
		ErrorLog.Println(logPrefix, "lookupCompanyByShortname err: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Company not found"})
		return
	}

	var webclockSetting *WebClockSettings
	coProd, err := getCompanyProductByURL(co.ID, "webclock")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Company not found"})
		return
	}

	if coProd.Enabled != nil && !*coProd.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Company not found"})
		return
	}

	clientIP := c.GetHeader("X-Real-IP")
	if !env.Production {
		clientIP = "0.0.0.0"
	}

	webclockSetting = coProd.Settings.ToWebclockSetting()
	if !webclockSetting.IPSetting.AllowAll {
		allowed := false
		for _, ip := range webclockSetting.IPSetting.AllowedIPs {
			if clientIP == ip {
				allowed = true
				break
			}
		}

		if !allowed {
			ErrorLog.Println(logPrefix, "tried punch from disallowed IP ", clientIP)
			statement := "Your company prohibits punches from your current public IP Address of " + clientIP
			c.JSON(http.StatusBadRequest, gin.H{"error": statement})
			return
		}
	}

	if webclockSetting.LocationByIP.Active {
		cxn := chooseOPAPICxn(co.OPID)
		ccs, err := getAllCostCentersWithCache(cxn, &co, false)
		if err != nil {
			ErrorLog.Println(logPrefix, " err looking up companyID: ", co.OPID, " GetAllCompanyCostCenters: ", err)
		} else {
			if ccs != nil {
				for _, ccTree := range *ccs {
					if ccTree.Name == "Location" {
						found := false

						for _, cc := range ccTree.Values {
							if cc.ExternalID == clientIP {
								newCCs := []WebClockCCArg{}

								if input.CostCenters != nil {
									// remove this index from ccs so we can override
									for _, c := range *input.CostCenters {
										if c.Index != ccTree.Index {
											newCCs = append(newCCs, c)
										}
									}
								}

								newCCs = append(newCCs, WebClockCCArg{Index: ccTree.Index, Value: cc})

								input.CostCenters = &newCCs

								found = true
								break
							}
						}

						if !found && webclockSetting.LocationByIP.Required {
							ErrorLog.Println(logPrefix, "tried punch from disallowed IP ", clientIP)
							statement := "Your company prohibits punches from your current public IP Address of " + clientIP
							c.JSON(http.StatusBadRequest, gin.H{"error": statement})
							return
						}

						break
					}
				}
			}
		}
	}

	if input.Username != "" && input.Password != "" {
		// if these were sent, we need to use v1

		opCreds := opapi.OPCredentials{
			Username:         input.Username,
			Password:         input.Password,
			CompanyShortname: co.ShortName,
			APIKey:           co.APIKey,
		}

		opCxn := &opapi.OPConnection{
			Credentials: &opCreds,
		}

		err = opCxn.LoginAndSetToken()
		if err != nil {
			ErrorLog.Println(logPrefix, "LoginAndSetToken err: ", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		punch := &opapi.V1TimePunchRequest{Action: input.PunchType}

		if input.PunchType == "change_ccs" && input.CostCenters != nil {
			for _, cc := range *input.CostCenters {
				switch cc.Index {
				case 0:
					punch.CostCenter1 = &opapi.V1TimePunchCC{Name: cc.Value.Name}
				case 1:
					punch.CostCenter2 = &opapi.V1TimePunchCC{Name: cc.Value.Name}
				case 2:
					punch.CostCenter3 = &opapi.V1TimePunchCC{Name: cc.Value.Name}
				case 3:
					punch.CostCenter4 = &opapi.V1TimePunchCC{Name: cc.Value.Name}
				case 4:
					punch.CostCenter5 = &opapi.V1TimePunchCC{Name: cc.Value.Name}
				case 5:
					punch.CostCenter6 = &opapi.V1TimePunchCC{Name: cc.Value.Name}
				case 6:
					punch.CostCenter7 = &opapi.V1TimePunchCC{Name: cc.Value.Name}
				case 7:
					punch.CostCenter8 = &opapi.V1TimePunchCC{Name: cc.Value.Name}
				case 8:
					punch.CostCenter9 = &opapi.V1TimePunchCC{Name: cc.Value.Name}
				case 9:
					punch.CostCenter10 = &opapi.V1TimePunchCC{Name: cc.Value.Name}
				}
			}
		}

		resp, err, _ := opCxn.SendTimePunchesV1(punch)
		if err != nil {
			ErrorLog.Println(logPrefix, "SendTimePunchesV1 err: ", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "An error occurred"})
			return
		}

		if resp.Status == "FAILURE" {
			if len(resp.Errors) > 0 {
				ErrorLog.Println(logPrefix, "SendTimePunchesV1 FAILURE: ", resp.Errors)
				c.JSON(http.StatusBadRequest, gin.H{"error": resp.Errors[0].Message})
				return
			}
			ErrorLog.Println(logPrefix, "SendTimePunchesV1 FAILURE but no returned errors")
			c.JSON(http.StatusBadRequest, gin.H{"error": "An unknown error occurred"})
			return
		}

		response := gin.H{"message": "Punch successful"}

		var timeData *opapi.EmployeeTimeEntries
		if input.IsFinalOut != nil {
			if *input.IsFinalOut == true {
				if webclockSetting != nil {
					if webclockSetting.Attestation.Active {
						timeData, err = getEmployeeTimeEntries(&co, input, nil, 0, false)
						if err != nil {
							ErrorLog.Println(logPrefix, "getEmployeeTimeEntries error:", err)
						} else {
							response["time_data"] = timeData
						}
					}
				}
			}
		}

		if input.Photo != nil && *input.Photo != "" {
			go webPunchPostProcess(&co, input, resp.Punch.Type, resp.Punch.Timestamp, nil, timeData)
		}

		// deprecated
		// if webclockSetting.ShowEmployeeName.Active {
		// 	emp, err := findEmployeeByUsernameOrEmployeeID(&input.Username, nil, &co)
		// 	if err == nil {
		// 		response["employee_name"] = emp.FirstName + " " + emp.LastName
		// 	}
		// }

		c.JSON(http.StatusOK, response)
	} else if input.EmployeeID != "" {
		employee, err := findEmployeeByUsernameOrEmployeeID(nil, &input.EmployeeID, &co)
		if err != nil {
			ErrorLog.Println(logPrefix, err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Employee not found"})
			return
		}

		if employee == nil {
			ErrorLog.Println(logPrefix, "employee not found")
			c.JSON(http.StatusBadRequest, gin.H{"error": "Employee not found"})
			return
		}

		// we must use specific timezone, because when looking for time in timesheet they send it in this zone
		// the problem is, when using v1 time punch, it returns the timestamp from the server
		// where as v2 does not, so we must use what we sent

		// alternative solution is to request the most recent punch from the api

		// this is still going to fail if Kronos sends back a different timezone since we compare strings
		loc, err := time.LoadLocation("America/Los_Angeles")
		if err != nil {
			ErrorLog.Println(logPrefix, "timezone error:", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "An error occured"})
			return
		}

		timestamp := time.Now().In(loc).Format(timestampFormat)
		if input.Timestamp != "" {
			timestamp = input.Timestamp
		}

		punchType := ""
		switch input.PunchType {
		case "punch_in":
			punchType = "PUNCH_IN"
		case "punch_out":
			punchType = "PUNCH_OUT"
		case "punch":
			punchType = "PUNCH"
		case "change_ccs":
			punchType = "CHANGE_CCS"
		}
		punch := opapi.TimePunch{
			Employee: opapi.TimePunchEmployee{
				AccountID: employee.ID,
			},
			RawType:   punchType,
			Timestamp: timestamp,
		}

		if input.CostCenters != nil {
			for _, cc := range *input.CostCenters {
				punch.CostCenters = append(punch.CostCenters, opapi.TimePunchCC{Index: cc.Index, Value: opapi.TimePunchCCID{ID: cc.Value.ID}})
			}
		}

		req := opapi.TimePunchRequest{Punches: []opapi.TimePunch{punch}}

		cxn := chooseOPAPICxn(co.OPID)
		_, resp, err, opError := cxn.SendTimePunches(co.OPID, &req)
		if err != nil {
			if opError != nil {
				ErrorLog.Printf("PUNCH ERR | %v / %v calls after | opError: %+v\n", cxn.CallLimit.CurrentCalls, cxn.CallLimit.Threshold, opError.Errors)
				c.JSON(http.StatusBadRequest, gin.H{"error": opError.String()})
				return
			} else {
				ErrorLog.Printf("PUNCH ERR | %v / %v calls after | INTERNAL err: %s\n", cxn.CallLimit.CurrentCalls, cxn.CallLimit.Threshold, err.Error())
				c.JSON(http.StatusBadRequest, gin.H{"error": "An error occurred"})
				return
			}
		}

		if len(resp.Items) > 0 && resp.Items[0].Status == "FAILURE" && len(resp.Items[0].Errors) > 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": resp.Items[0].Errors[0].Message})
			return
		}

		InfoLog.Printf("%s SENT PUNCH | %v / %v calls after\n", logPrefix, cxn.CallLimit.CurrentCalls, cxn.CallLimit.Threshold)

		response := gin.H{"message": "Punch successful"}

		var timeData *opapi.EmployeeTimeEntries
		if input.IsFinalOut != nil {
			if *input.IsFinalOut == true {
				if webclockSetting != nil {
					if webclockSetting.Attestation.Active {
						timeData, err = getEmployeeTimeEntries(&co, input, employee, 0, false)
						if err != nil {
							ErrorLog.Println(logPrefix, "getEmployeeTimeEntries error:", err)
						} else {
							response["time_data"] = timeData
						}
						// also give the time entries to webPunchPostProcess
					}
				}
			}
		}

		if input.Photo != nil && *input.Photo != "" {
			go webPunchPostProcess(&co, input, punchType, timestamp, employee, timeData)
		}

		// deprecated
		// if webclockSetting.ShowEmployeeName.Active {
		// 	response["employee_name"] = employee.FirstName + " " + employee.LastName
		// }

		c.JSON(http.StatusOK, response)
	}
}

// daysBack: days ago you want start date to be. Useful to get TEs from yesterday and today to ensure you get all
func getEmployeeTimeEntries(company *Company, request *WebClockRequest, employee *opapi.OPFullEmployeeRecord, daysBack int, isLight bool) (*opapi.EmployeeTimeEntries, error) {
	InfoLog.Println("requesting TIME ENTRIES FROM OPAPI")
	var err error
	if employee == nil {
		if request.Username != "" {
			employee, err = findEmployeeByUsernameOrEmployeeID(&request.Username, nil, company)
		} else if request.EmployeeID != "" {
			employee, err = findEmployeeByUsernameOrEmployeeID(nil, &request.EmployeeID, company)
		}
		if err != nil {
			return nil, errors.New(err.Error())
		}
	}

	if employee == nil {
		return nil, errors.New(err.Error())
	}

	adminCxn := chooseOPAPICxn(company.OPID)

	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return nil, errors.New(err.Error())
	}

	today := time.Now().In(loc)
	start := today.AddDate(0, 0, -(daysBack))

	todayStr := today.Format("2006-01-02")
	startStr := start.Format("2006-01-02")
	allTimeEntries, err := adminCxn.GetEmployeeTimeEntries(int64(employee.ID), company.OPID, startStr, todayStr, false)
	if err != nil {
		return nil, errors.New(err.Error())
	}

	return &allTimeEntries, nil
}

func webPunchPostProcess(company *Company, request *WebClockRequest, punchTypeUpperCase, timestamp string, employee *opapi.OPFullEmployeeRecord, timeData *opapi.EmployeeTimeEntries) {
	logPrefix := fmt.Sprintf("webClock postprocess [%s %s %s] ", request.CompanyShortname, request.Username, request.PunchType)

	photoFileName, err := request.uploadPhoto()
	if err != nil {
		ErrorLog.Println(logPrefix, err)
		return
	}

	if timeData == nil {
		timeData, err = getEmployeeTimeEntries(company, request, employee, 1, false)
		if err != nil {
			ErrorLog.Println(logPrefix, err)
			return
		}
	}

	timeEntriesToSend := []opapi.TimeEntry{}
	for _, timeEntry := range timeData.TimeEntries {
		if (timeEntry.StartTime == nil || *timeEntry.StartTime == "") && (timeEntry.EndTime == nil || *timeEntry.EndTime == "") && (timeEntry.Total == nil) {
			// exclude because the update will reject these
			continue
		}

		// change index 3 for IN index 4 for OUT
		var customIndexToUpdate *int
		if timeEntry.StartTime != nil {
			if *timeEntry.StartTime == timestamp {
				newVal := 3
				customIndexToUpdate = &newVal
			}
		}

		if timeEntry.EndTime != nil {
			if *timeEntry.EndTime == timestamp {
				newVal := 4
				customIndexToUpdate = &newVal
			}
		}

		if customIndexToUpdate != nil {
			newCustomFields := []opapi.TimeEntryCustomFields{}
			if timeEntry.CustomFields != nil {
				for _, cusField := range *timeEntry.CustomFields {
					if cusField.Index != *customIndexToUpdate {
						newCustomFields = append(newCustomFields, cusField)
					}
				}
			}
			newCustomFields = append(newCustomFields, opapi.TimeEntryCustomFields{Index: *customIndexToUpdate, Value: photoFileName})
			timeEntry.CustomFields = &newCustomFields
		}

		// make fields nil that UPDATE does not accept but the GET sends
		timeEntry.ID = nil
		timeEntry.CalcPayCategory = nil
		timeEntry.CalcStartTime = nil
		timeEntry.CalcEndTime = nil
		timeEntry.CalcTotal = nil
		timeEntry.IsRaw = nil
		timeEntry.IsCalc = nil

		timeEntriesToSend = append(timeEntriesToSend, timeEntry)
	}

	timeData.TimeEntries = timeEntriesToSend

	updateBody := opapi.TimeEntriesUpdateRequest{TimeEntrySets: []opapi.EmployeeTimeEntries{*timeData}}

	adminCxn := chooseOPAPICxn(company.OPID)
	_, opListResponse, err, opError := adminCxn.UpdateTimeEntries(company.OPID, &updateBody)
	if err != nil {
		if opError != nil {
			ErrorLog.Printf(logPrefix, "opError: %+v\n", opError.Errors)
			return
		} else {
			ErrorLog.Printf(logPrefix, "err: %s\n", err.Error())
			return
		}
	}

	if len(opListResponse.Items) > 0 {
		if opListResponse.Items[0].Status == "FAILURE" {
			ErrorLog.Printf("%s opListResponse: %+v\n", logPrefix, opListResponse)
			return
		}
	} else {
		ErrorLog.Println(logPrefix, "len(opListResponse.Items) was 0")
		return
	}

	record := &WebClockPunch{
		CompanyID:      company.ID,
		AccountID:      int64(employee.ID),
		PunchType:      punchTypeUpperCase,
		PhotoLink:      photoFileName,
		PunchTimestamp: timestamp,
	}
	dbmap.Insert(record)

	InfoLog.Println("webClock postprocess SUCCESSFUL")
}

func removeLeadingZeroes(str string) string {
	i := 0
	for i < len(str) && str[i] == '0' {
		i++
	}

	return str[i:]
}

func findEmployeeByUsernameOrEmployeeID(username, employeeID *string, company *Company) (*opapi.OPFullEmployeeRecord, error) {
	emps, err := getAllCompanyEmployeesWithCache(company, false, true)
	if err != nil {
		return nil, err
	}

	if emps == nil {
		return nil, errors.New("no employees found")
	}

	var foundEmp *opapi.OPFullEmployeeRecord
	for _, emp := range *emps {
		if username != nil {
			if strings.ToLower(emp.Username) == strings.ToLower(*username) {
				foundEmp = &emp
				break
			}
		} else if employeeID != nil {
			// cut leading zeros of both
			if removeLeadingZeroes(strings.ToLower(emp.EmployeeID)) == removeLeadingZeroes(strings.ToLower(*employeeID)) {
				foundEmp = &emp
				break
			}
		}
	}

	if foundEmp == nil {
		return nil, errors.New("employee not found")
	}

	return foundEmp, nil
}

func (request *WebClockRequest) uploadPhoto() (string, error) {
	randomStr := GenRandStr(50)
	filename := fmt.Sprintf("%s%d.png", randomStr, time.Now().Unix())

	// assumes photo comes in base64 encoded string in form of "data:image/png;base64,iVBORw0KGgo..."
	commaI := strings.Index(*request.Photo, ",")
	rawImage := (*request.Photo)[commaI+1:]

	pBytes, err := base64.StdEncoding.DecodeString(rawImage)
	if err != nil {
		return "", err
	}

	err = bytesToGCP(PUNCH_PHOTOS_BUCKETNAME, filename, pBytes, true)
	if err != nil {
		return "", err
	}

	return PUNCH_PHOTOS_URL + filename, nil
}

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890"

var src = rand.NewSource(time.Now().UnixNano())

const (
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

func GenRandStr(n int) string {
	b := make([]byte, n)
	// A src.Int63() generates 63 random bits, enough for letterIdxMax characters!
	for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			b[i] = letterBytes[idx]
			i--
		}
		cache >>= letterIdxBits
		remain--
	}

	return *(*string)(unsafe.Pointer(&b))
}

func (pm *PropertyMap) ToWebclockSetting() *WebClockSettings {
	var wcSetting WebClockSettings
	datab, err := json.Marshal(pm)
	if err != nil {
		ErrorLog.Println(err)
		return nil
	}
	err = json.Unmarshal(datab, &wcSetting)
	if err != nil {
		ErrorLog.Println(err)
		return nil
	}

	return &wcSetting
}
