package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type CPChange struct {
	ID        int64        `db:"id, primarykey, autoincrement" json:"id"`
	CompanyID int64        `db:"company_id" json:"company_id"`
	Type      string       `db:"type" json:"type"`
	Data      *PropertyMap `db:"data,size:10000" json:"data"`
}

type CPAddEnrollmentDB struct {
	EmployeeAccountID int64 `json:"employee_account_id"`
	EnrollmentID      int64 `json:"enrollment_id"`
}

type CPDeleteEnrollmentDB struct {
	GlobalID int64 `json:"global_id"`
}

type CPEmployeeChangeDB struct {
	AccountID int64 `json:"account_id"`
}

type CPUpdateFiltersDB struct {
	InstallationID int64 `json:"installation_id"`
}

type CPAddEnrollmentSync struct {
	Enrollment CPEnrollment `json:"enrollment"`
	Employee   CPEmployee   `json:"employee"`
}

type CPAddEmployeeSync struct {
	Employee []CPEmployee `json:"employees"`
}

type CPChangeResponse struct {
	ID   int64        `json:"id"`
	Type string       `json:"type"`
	Data *PropertyMap `json:"data"`
}

const (
	CPCHANGETYPE_ADD_EMPLOYEE          = "ADD_EMPLOYEE"
	CPCHANGETYPE_DELETE_EMPLOYEE       = "DELETE_EMPLOYEE"
	CPCHANGETYPE_ADD_ENROLLMENT        = "ADD_ENROLLMENT"
	CPCHANGETYPE_DELETE_ENROLLMENT     = "DELETE_ENROLLMENT"
	CPCHANGETYPE_UPDATE_INSTALL_FILTER = "UPDATE_INSTALL_FILTER"
	CPCHANGETYPE_SKIP                  = "SKIP"
)

func (emp *CPEmployee) ProcessToggleActive(newActive bool) {
	// can optionally update cost center info here

	data := &CPEmployeeChangeDB{
		AccountID: emp.AccountID,
	}

	newChange := &CPChange{
		CompanyID: emp.CompanyID,
		Data:      data.ToPropertyMap(),
	}

	if newActive {
		newChange.Type = CPCHANGETYPE_ADD_EMPLOYEE
	} else {
		newChange.Type = CPCHANGETYPE_DELETE_EMPLOYEE
	}

	dbmap.Insert(newChange)
}

func getCPSyncHandler(c *gin.Context) {
	thisCompany, err := lookupCompany(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	installTokenParam := c.Query("installToken")
	if installTokenParam == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Install token is a required parameter"})
		return
	}

	installation, err := getInstallationByToken(installTokenParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Installation not found"})
		return
	}

	if installation.CompanyID != thisCompany.ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	syncMarkerParam := c.Query("marker")
	if syncMarkerParam == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Marker is a required parameter"})
		return
	}

	syncMarker, err := strconv.Atoi(syncMarkerParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Marker is a required parameter"})
		return
	}

	changes, err := cpGetSyncChanges(int64(syncMarker), &thisCompany)
	if err != nil {
		ErrorLog.Println("cpGetSyncChanges err: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "An error occured"})
		return
	}

	skipAllFutureAddAndDeletes := false

	returnedChanges := []CPChangeResponse{}
	for _, dbChange := range changes {
		resp := CPChangeResponse{
			Type: dbChange.Type,
			ID:   dbChange.ID,
		}

		switch dbChange.Type {
		case CPCHANGETYPE_ADD_EMPLOYEE:
			if skipAllFutureAddAndDeletes {
				resp.Type = CPCHANGETYPE_SKIP
				resp.Data = nil
				returnedChanges = append(returnedChanges, resp)
				continue
			}

			changeInfoFromDB := dbChange.Data.ToCPEmployeeChangeDB()
			if changeInfoFromDB == nil {
				resp.Type = CPCHANGETYPE_SKIP
				resp.Data = nil
				returnedChanges = append(returnedChanges, resp)
				continue
			}

			emp, _ := getCPEmployeeByAccountID(changeInfoFromDB.AccountID)

			meetsFilter := emp.MeetsEmployeeFilter(installation.EmployeeFilters)
			if !meetsFilter {
				resp.Type = CPCHANGETYPE_SKIP
				resp.Data = nil
			} else {
				enrs, err := getCPEnrollmentsByEmployee(emp.AccountID, true, true)
				if err != nil {
					ErrorLog.Println("getCPEnrollmentsByEmployee err: ", err)
					continue
				}
				emp.Enrollments = &enrs
				resp.Data = emp.ToPropertyMap()
			}

			returnedChanges = append(returnedChanges, resp)
		case CPCHANGETYPE_DELETE_EMPLOYEE:
			if skipAllFutureAddAndDeletes {
				resp.Type = CPCHANGETYPE_SKIP
				resp.Data = nil
				returnedChanges = append(returnedChanges, resp)
				continue
			}

			changeInfoFromDB := dbChange.Data.ToCPEmployeeChangeDB()
			if changeInfoFromDB == nil {
				resp.Type = CPCHANGETYPE_SKIP
				resp.Data = nil
				returnedChanges = append(returnedChanges, resp)
				continue
			}

			emp, _ := getCPEmployeeByAccountID(changeInfoFromDB.AccountID)
			resp.Data = emp.ToPropertyMap()

			// just send to each installation

			returnedChanges = append(returnedChanges, resp)
		case CPCHANGETYPE_ADD_ENROLLMENT:
			if skipAllFutureAddAndDeletes {
				resp.Type = CPCHANGETYPE_SKIP
				resp.Data = nil
				returnedChanges = append(returnedChanges, resp)
				continue
			}

			changeInfoFromDB := dbChange.Data.ToCPAddEnrollmentDB()
			if changeInfoFromDB == nil {
				resp.Type = CPCHANGETYPE_SKIP
				resp.Data = nil
				returnedChanges = append(returnedChanges, resp)
				continue
			}

			emp, _ := getCPEmployeeByAccountID(changeInfoFromDB.EmployeeAccountID)

			meetsFilter := emp.MeetsEmployeeFilter(installation.EmployeeFilters)
			if !meetsFilter {
				resp.Type = CPCHANGETYPE_SKIP
				resp.Data = nil
			} else {
				beenDisabled, data := cpAddEnrollmentGatherData(changeInfoFromDB)
				if beenDisabled {
					resp.Type = CPCHANGETYPE_SKIP
					resp.Data = nil
				} else {
					resp.Data = data
				}
			}

			returnedChanges = append(returnedChanges, resp)
		case CPCHANGETYPE_DELETE_ENROLLMENT:
			if skipAllFutureAddAndDeletes {
				resp.Type = CPCHANGETYPE_SKIP
				resp.Data = nil
				returnedChanges = append(returnedChanges, resp)
				continue
			}

			resp.Data = dbChange.Data
			returnedChanges = append(returnedChanges, resp)
		case CPCHANGETYPE_UPDATE_INSTALL_FILTER:
			if skipAllFutureAddAndDeletes {
				resp.Type = CPCHANGETYPE_SKIP
				resp.Data = nil
				returnedChanges = append(returnedChanges, resp)
				continue
			}

			updateData := dbChange.Data.ToCPUpdateFiltersDB()

			if installation.ID != updateData.InstallationID {
				resp.Type = CPCHANGETYPE_SKIP
				resp.Data = nil
				returnedChanges = append(returnedChanges, resp)
				continue
			}

			allEmps, err := getAllCompanyCPEmployees(thisCompany, true, true)
			if err != nil {
				ErrorLog.Println("getAllCompanyCPEmployees err: ", err)
				break
			}

			filteredEmployees := []CPEmployee{}
			for _, emp := range allEmps {
				if emp.MeetsEmployeeFilter(installation.EmployeeFiltersPM.ToCPEmployeeFilters()) {
					enrs, err := getCPEnrollmentsByEmployee(emp.AccountID, true, true)
					if err != nil {
						ErrorLog.Println("getCPEnrollmentsByEmployee err: ", err)
						continue
					}
					emp.Enrollments = &enrs
					filteredEmployees = append(filteredEmployees, emp)
				}
			}

			respData := CPAddEmployeeSync{filteredEmployees}
			data := respData.ToPropertyMap()
			resp.Data = data
			returnedChanges = append(returnedChanges, resp)
			skipAllFutureAddAndDeletes = true
		}
	}

	c.JSON(http.StatusOK, returnedChanges)

	go installation.UpdateLastSyncedAndIP(time.Now(), c.GetHeader("X-Real-IP"))
}

func (e *CPEmployee) MeetsEmployeeFilter(filter *CPEmployeeFilters) bool {
	if !e.Active {
		return false
	}

	indexToDefaultCCMap := make(map[int]int64)
	indexToDefaultCCMap[0] = e.CostCenter0
	indexToDefaultCCMap[1] = e.CostCenter1
	indexToDefaultCCMap[2] = e.CostCenter2
	indexToDefaultCCMap[3] = e.CostCenter3
	indexToDefaultCCMap[4] = e.CostCenter4

	for _, ccFilter := range filter.CostCenters.Indexes {
		defaultCCID := indexToDefaultCCMap[ccFilter.Index]
		for _, cc := range ccFilter.Values {
			if cc.CostCenterID == defaultCCID {
				return true
			}
		}
	}
	return false
}

func cpGetSyncChanges(currentMarker int64, company *Company) ([]CPChange, error) {
	changes := []CPChange{}
	_, err := dbmap.Select(&changes, "SELECT * FROM cp_changes WHERE company_id = ? AND id > ?", company.ID, currentMarker)
	return changes, err
}

func cpCreateNewAddEnrollmentChange(enrollment *CPEnrollment) error {
	data := CPAddEnrollmentDB{
		EmployeeAccountID: enrollment.EmployeeAccountID,
		EnrollmentID:      enrollment.ID,
	}

	var dataPM PropertyMap
	datab, err := json.Marshal(data)
	if err != nil {
		return err
	}
	err = json.Unmarshal(datab, &dataPM)
	if err != nil {
		return err
	}

	newChange := &CPChange{
		CompanyID: enrollment.CompanyID,
		Type:      CPCHANGETYPE_ADD_ENROLLMENT,
		Data:      &dataPM,
	}

	err = dbmap.Insert(newChange)

	return err
}

func cpCreateEnrollmentDeletionChange(enrollment *CPEnrollment) error {
	data := CPDeleteEnrollmentDB{
		GlobalID: enrollment.ID,
	}

	var dataPM PropertyMap
	datab, err := json.Marshal(data)
	if err != nil {
		return err
	}
	err = json.Unmarshal(datab, &dataPM)
	if err != nil {
		return err
	}

	newChange := &CPChange{
		CompanyID: enrollment.CompanyID,
		Type:      CPCHANGETYPE_DELETE_ENROLLMENT,
		Data:      &dataPM,
	}

	err = dbmap.Insert(newChange)

	return err
}

// Returns beenDisabled and required data as a property map
func cpAddEnrollmentGatherData(dbData *CPAddEnrollmentDB) (bool, *PropertyMap) {
	beenDisabled := false

	enr, _ := getCPEnrollmentByID(dbData.EnrollmentID)
	emp, _ := getCPEmployeeByAccountID(dbData.EmployeeAccountID)

	dataNeeded := CPAddEnrollmentSync{
		Enrollment: *enr,
		Employee:   *emp,
	}

	if !enr.Active {
		beenDisabled = true
	}

	var dataNeededPM PropertyMap
	datab, err := json.Marshal(dataNeeded)
	if err != nil {
		return beenDisabled, nil
	}
	err = json.Unmarshal(datab, &dataNeededPM)
	if err != nil {
		return beenDisabled, nil
	}

	return beenDisabled, &dataNeededPM
}

func (pm *PropertyMap) ToCPAddEnrollmentDB() *CPAddEnrollmentDB {
	var changeInfoFromDB CPAddEnrollmentDB
	datab, err := json.Marshal(pm)
	if err != nil {
		return nil
	}
	err = json.Unmarshal(datab, &changeInfoFromDB)
	if err != nil {
		return nil
	}

	return &changeInfoFromDB
}

func (pm *PropertyMap) ToCPUpdateFiltersDB() *CPUpdateFiltersDB {
	var changeInfoFromDB CPUpdateFiltersDB
	datab, err := json.Marshal(pm)
	if err != nil {
		return nil
	}
	err = json.Unmarshal(datab, &changeInfoFromDB)
	if err != nil {
		return nil
	}

	return &changeInfoFromDB
}

func (pm *PropertyMap) ToCPEmployeeFilters() *CPEmployeeFilters {
	var cpfilters CPEmployeeFilters
	datab, err := json.Marshal(pm)
	if err != nil {
		return nil
	}
	err = json.Unmarshal(datab, &cpfilters)
	if err != nil {
		return nil
	}

	return &cpfilters
}

func (cf *CPEmployeeFilters) ToPropertyMap() *PropertyMap {
	var pm PropertyMap
	datab, err := json.Marshal(cf)
	if err != nil {
		return nil
	}
	err = json.Unmarshal(datab, &pm)
	if err != nil {
		return nil
	}

	return &pm
}

func (cf *CPAddEmployeeSync) ToPropertyMap() *PropertyMap {
	var pm PropertyMap
	datab, err := json.Marshal(cf)
	if err != nil {
		return nil
	}
	err = json.Unmarshal(datab, &pm)
	if err != nil {
		return nil
	}

	return &pm
}

func (cpe *CPEmployee) ToPropertyMap() *PropertyMap {
	var pm PropertyMap
	datab, err := json.Marshal(cpe)
	if err != nil {
		return nil
	}
	err = json.Unmarshal(datab, &pm)
	if err != nil {
		return nil
	}

	return &pm
}

func (cf *CPEmployeeChangeDB) ToPropertyMap() *PropertyMap {
	var pm PropertyMap
	datab, err := json.Marshal(cf)
	if err != nil {
		return nil
	}
	err = json.Unmarshal(datab, &pm)
	if err != nil {
		return nil
	}

	return &pm
}

func (pm *PropertyMap) ToCPEmployeeChangeDB() *CPEmployeeChangeDB {
	var cpe CPEmployeeChangeDB
	datab, err := json.Marshal(pm)
	if err != nil {
		return nil
	}
	err = json.Unmarshal(datab, &cpe)
	if err != nil {
		return nil
	}

	return &cpe
}
