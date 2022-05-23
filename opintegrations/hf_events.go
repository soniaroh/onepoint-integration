package main

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

type HFEvent struct {
	ID          int64    `db:"id, primarykey, autoincrement" json:"id"`
	Type        int8     `db:"type" json:"type"`
	OPAccountID int64    `db:"op_account_id" json:"-"`
	EventDate   string   `db:"event_date" json:"event_date"`
	CompanyID   int64    `db:"company_id" json:"-"`
	FirstName   string   `db:"first_name" json:"first_name"`
	LastName    string   `db:"last_name" json:"last_name"`
	Email       string   `db:"email" json:"email"`
	Processed   *bool    `db:"processed" json:"-"`
	Created     int64    `db:"created" json:"-"`
	Updated     *int64   `db:"updated" json:"-"`
	Result      HFResult `db:"-" json:"result"`
}

type HFResult struct {
	ID                   int64  `db:"id, primarykey, autoincrement" json:"-"`
	CompanyIntegrationID int64  `db:"company_integration_id" json:"-"`
	Status               string `db:"status" json:"status"`
	HFEventID            int64  `db:"hf_event_id" json:"-"`
	Description          string `db:"description" json:"description"`
	Created              int64  `db:"created" json:"-"`
	Updated              *int64 `db:"updated" json:"-"`
}

type ByDateDesc []HFEvent

func (a ByDateDesc) Len() int      { return len(a) }
func (a ByDateDesc) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByDateDesc) Less(i, j int) bool {
	iDate, _ := time.Parse("01/02/2006", a[i].EventDate)
	jDate, _ := time.Parse("01/02/2006", a[j].EventDate)
	if iDate.Unix() == jDate.Unix() {
		return a[i].Created > a[j].Created
	}
	return iDate.Unix() > jDate.Unix()
}

type HFEventsResponse struct {
	IntegrationURL string    `json:"url"`
	Events         []HFEvent `json:"events"`
}

const (
	HF_LAST_PING_SETTING = "lastPinged"
)

const (
	HIRED_TYPE_ID   = 1
	FIRED_TYPE_ID   = 2
	CHANGED_TYPE_ID = 3
)

const (
	HF_SUCCESS_LABEL    = "SUCCESS"
	HF_FAILED_LABEL     = "FAIL"
	HF_DISABLED_LABEL   = "DISABLED"
	HF_CC_BLOCKED_LABEL = "COSTCNTR"
	HF_JT_BLOCKED_LABEL = "JOBTITLE"
	HF_PENDING_LABEL    = "PENDING"
	HF_OVERRIDDEN       = "OVERRIDDEN"
)

func registerEventRoutes(router *gin.Engine) {
	router.GET("/api/integrations/:intURL/events", getCompanysEventsHandler)
	router.GET("/api/pendingActions/:intURL", getUserProvisioningPendingActionsHandler)
	router.POST("/api/pendingActions/:intURL/ack", ackUserProvisioningHandler)
}

func getCompanysEventsHandler(c *gin.Context) {
	thisCompany, err := lookupCompany(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	intURL := c.Param("intURL")

	companyIntegration, err := getCompanyIntegrationByIntegrationString(intURL, thisCompany.ID)
	if err != nil {
		ErrorLog.Println("getCompanyIntegrationByIntegrationString cant find this company integration: ", err)
		c.JSON(400, gin.H{"error": "Not found"})
		return
	}

	allEvents := []HFEvent{}
	_, err = dbmap.Select(&allEvents, "SELECT * FROM hf_events WHERE company_id = ? AND processed = 1 AND created > ? ORDER BY STR_TO_DATE(event_date,'%m/%d/%Y') DESC LIMIT 300", thisCompany.ID, companyIntegration.Created)
	if err != nil {
		ErrorLog.Println("cant lookup events: ", err)
		c.JSON(400, gin.H{"error": "Not found"})
		return
	}

	eventsWeWant := []HFEvent{}
	for index := 0; index < len(allEvents); index++ {
		result := HFResult{}
		err = dbmap.SelectOne(&result, "SELECT * FROM hf_results WHERE hf_event_id = ? AND company_integration_id = ?", allEvents[index].ID, companyIntegration.ID)
		if err != nil {
			if allEvents[index].Type == CHANGED_TYPE_ID {
				continue
			}
			result.HFEventID = allEvents[index].ID
			result.CompanyIntegrationID = companyIntegration.ID
			result.Status = "N/A"
			result.Description = "This integration did not do anything for this event."
		}

		if allEvents[index].Email == "" {
			allEvents[index].Email = "No Primary Email at time of event"
		}

		allEvents[index].Result = result
		eventsWeWant = append(eventsWeWant, allEvents[index])
	}

	sort.Sort(ByDateDesc(eventsWeWant))

	c.JSON(200, HFEventsResponse{IntegrationURL: intURL, Events: eventsWeWant})
}

type HFAction struct {
	ID           int64        `json:"id"`
	Type         string       `json:"type"`
	User         HFActionUser `json:"user"`
	CopyUsername *string      `json:"copy_username,omitempty"`
}

type HFActionUser struct {
	ID          int    `json:"id"`
	Username    string `json:"username"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	EmployeeID  string `json:"employee_id"`
	Email       string `json:"primary_email"`
	JobTitle    string `json:"job_title"`
	Address     string `json:"address"`
	OfficePhone string `json:"office_phone"`
	MobilePhone string `json:"mobile_phone"`
	Password    string `json:"password"`
}

const (
	HF_ACTION_TYPE_ADD    = "ADD"
	HF_ACTION_TYPE_DELETE = "DELETE"
	HF_ACTION_TYPE_MODIFY = "MODIFY"
)

func getUserProvisioningPendingActionsHandler(c *gin.Context) {
	InfoLog.Println(c.Request.Header)

	thisCompany, err := lookupCompany(c)
	if err != nil {
		ErrorLog.Println("getUserProvisioningPendingActionsHandler lookupCompany err: ", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	intURL := c.Param("intURL")

	companyIntegration, err := getCompanyIntegrationByIntegrationString(intURL, thisCompany.ID)
	if err != nil {
		ErrorLog.Println("getUserProvisioningPendingActionsHandler getCompanyIntegrationByIntegrationString cant find this company integration: ", err)
		c.JSON(400, gin.H{"error": "Integration not found"})
		return
	}

	nowSecs := time.Now().Unix()
	if companyIntegration.Status != "connected" {
		companyIntegration.Status = "connected"
	}
	companyIntegration.Settings[HF_LAST_PING_SETTING] = nowSecs
	_, err = dbmap.Update(&companyIntegration)
	if err != nil {
		ErrorLog.Println("err updating company int status and seconds")
	}

	allPendingResults := []HFResult{}
	_, err = dbmap.Select(&allPendingResults, "SELECT * FROM hf_results WHERE company_integration_id = ? AND status = ?", companyIntegration.ID, HF_PENDING_LABEL)
	if err != nil {
		ErrorLog.Println("cant lookup results: ", err)
		c.JSON(400, gin.H{"error": "An error occured: 3044"})
		return
	}

	cxn := chooseOPAPICxn(thisCompany.OPID)

	allActions := []HFAction{}
	for i, result := range allPendingResults {
		InfoLog.Println("starting pending: ", i)

		thisHFEvent := HFEvent{}
		err := dbmap.SelectOne(&thisHFEvent, "SELECT * FROM hf_events WHERE id = ?", result.HFEventID)
		if err != nil {
			ErrorLog.Printf("could not lookup hf event: %d, err: %v\n", result.HFEventID, err)
			c.JSON(400, gin.H{"error": "An unexpected error occured"})
			return
		}

		fullEmployeeRecord, err := cxn.GetFullEmployeeInfo(thisHFEvent.OPAccountID, thisCompany.OPID)
		if err != nil {
			ErrorLog.Printf("could not lookup hf employee: %d, err: %v\n", thisHFEvent.OPAccountID, err)
			c.JSON(400, gin.H{"error": "An unexpected error occured"})
			return
		}

		aType := ""
		switch thisHFEvent.Type {
		case HIRED_TYPE_ID:
			aType = HF_ACTION_TYPE_ADD
		case FIRED_TYPE_ID:
			aType = HF_ACTION_TYPE_DELETE
		case CHANGED_TYPE_ID:
			aType = HF_ACTION_TYPE_MODIFY
		}

		var adUsername *string
		var jobTitlePtr *string
		if aType == HF_ACTION_TYPE_ADD {
			adUsername, jobTitlePtr, err = checkHFTemplateMapping(thisCompany.OPID, companyIntegration, fullEmployeeRecord)
			if err != nil {
				if err.Error() != "jobnotfound" {
					ErrorLog.Println("AD checkHFTemplateMappingBlocked err: ", err)
					continue
				}
			}
			if adUsername == nil {
				ErrorLog.Println("NO AD USERNAME")
				reason := "The new employee was hired without being given a Job, therefore there is no template to build an Active User from."
				if jobTitlePtr != nil {
					reason = fmt.Sprintf("The new employee was given the Job Title of %s, which is not assigned an Active Directory Template.", *jobTitlePtr)
				}

				result.Description = reason
				result.Status = HF_JT_BLOCKED_LABEL
				updated := time.Now().Unix()
				result.Updated = &updated

				_, err = dbmap.Update(&result)
				if err != nil {
					ErrorLog.Println("cant Update hf_results: ", err)
				}

				continue
			}
		}

		if aType == HF_ACTION_TYPE_MODIFY {
			_, jobTitlePtr, err = checkHFTemplateMapping(thisCompany.OPID, companyIntegration, fullEmployeeRecord)
			if err != nil {
				ErrorLog.Println("checkHFTemplateMapping err: " + err.Error())
			}
		}

		jobTitle := ""
		if jobTitlePtr != nil {
			jobTitle = *jobTitlePtr
		}

		newPassword, err := getNewPasswordFromSetting(companyIntegration)
		if err != nil {
			ErrorLog.Println("getNewPasswordFromSetting error: " + err.Error())
		}

		newEmail, err := generateEmailAddressBasedOnSettings(fullEmployeeRecord, companyIntegration, []HFCompanyIntegrationResultJoin{})
		if err != nil {
			ErrorLog.Println("generateEmailAddressBasedOnSettings error: " + err.Error())
		}

		email := newEmail
		if newEmail == "" {
			email = fullEmployeeRecord.PrimaryEmail
		}

		// TODO: address needs to be company location

		// THIS IS A HOT FIX FOR WTF SINCE THEYRE ONLY USER
		fname := fullEmployeeRecord.FirstName
		if fullEmployeeRecord.Nickname != "" {
			fname = fullEmployeeRecord.Nickname
		}

		newAction := HFAction{
			ID:           result.ID,
			Type:         aType,
			CopyUsername: adUsername,
			User: HFActionUser{
				ID:          fullEmployeeRecord.ID,
				Username:    fullEmployeeRecord.Username,
				FirstName:   fname,
				LastName:    fullEmployeeRecord.LastName,
				EmployeeID:  fullEmployeeRecord.EmployeeID,
				Email:       email,
				JobTitle:    jobTitle,
				MobilePhone: fullEmployeeRecord.Phones.CellPhone,
				OfficePhone: fullEmployeeRecord.Phones.WorkPhone,
				Password:    newPassword,
			},
		}
		allActions = append(allActions, newAction)
	}

	c.JSON(200, allActions)
}

type UPAckBody struct {
	ActionID    int64        `json:"action_id"`
	Result      string       `json:"result"`
	Description string       `json:"description"`
	User        HFActionUser `json:"user"`
}

func ackUserProvisioningHandler(c *gin.Context) {
	thisCompany, err := lookupCompany(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	input := UPAckBody{}
	if err := c.ShouldBindWith(&input, binding.JSON); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	intURL := c.Param("intURL")
	coInt, err := getCompanyIntegrationByIntegrationString(intURL, thisCompany.ID)
	if err != nil {
		ErrorLog.Println("getUserProvisioningPendingActionsHandler getCompanyIntegrationByIntegrationString cant find this company integration: ", err)
		c.JSON(400, gin.H{"error": "Integration not found"})
		return
	}

	InfoLog.Printf("recieved USER PRO ACK inturl: %s,: %+v\n", intURL, input)

	thisResult := HFResult{}
	err = dbmap.SelectOne(&thisResult, "SELECT * FROM hf_results WHERE id=?", input.ActionID)
	if err != nil {
		ErrorLog.Println("cant lookup hf_results: ", err)
		c.JSON(400, gin.H{"error": "An error occured: 3044"})
		return
	}

	thisHFEvent := HFEvent{}
	err = dbmap.SelectOne(&thisHFEvent, "SELECT * FROM hf_events WHERE id = ?", thisResult.HFEventID)
	if err != nil {
		ErrorLog.Printf("could not lookup hf event: %d, err: %v\n", thisResult.HFEventID, err)
		c.JSON(400, gin.H{"error": "An unexpected error occured"})
		return
	}

	if input.Result == "SUCCESS" {
		thisResult.Status = HF_SUCCESS_LABEL
		if thisHFEvent.Type == HIRED_TYPE_ID {
			thisResult.Description = "User was successfully added to Active Directory."
		} else if thisHFEvent.Type == FIRED_TYPE_ID {
			thisResult.Description = "User was successfully deactivated from Active Directory."
		} else if thisHFEvent.Type == CHANGED_TYPE_ID {
			thisResult.Description = "User was successfully updated in Active Directory."
		}
	} else if input.Result == "FAIL" {
		thisResult.Status = HF_FAILED_LABEL
		thisResult.Description = fmt.Sprintf("An error occured: %s", input.Description)
	}

	updated := time.Now().Unix()
	thisResult.Updated = &updated

	_, err = dbmap.Update(&thisResult)
	if err != nil {
		ErrorLog.Println("cant Update hf_results: ", err)
		c.JSON(400, gin.H{"error": "An error occured: 3042"})
		return
	}

	go sendActiveDirectoryUserProvisioningEmail(thisHFEvent, thisResult, coInt, input.User)

	c.JSON(200, nil)
}
