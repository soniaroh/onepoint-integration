package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"opapi"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

type CPAllDataResponse struct {
	Installations []CPInstallation `json:"installations"`
	Employees     []CPEmployee     `json:"employees"`
}

type CPInstallation struct {
	ID                int64        `db:"id, primarykey, autoincrement" json:"id"`
	Token             string       `db:"token" json:"token"`
	CompanyID         int64        `db:"company_id" json:"company_id"`
	DisplayName       string       `db:"display_name" json:"display_name"`
	IP                string       `db:"ip" json:"ip"`
	LastSynced        string       `db:"last_synced" json:"last_synced"`
	EmployeeFiltersPM *PropertyMap `db:"employee_filters,size:10000" json:"employee_filters"`
	ClientSettingsPM  *PropertyMap `db:"client_settings,size:10000" json:"client_settings"`

	EmployeeFilters *CPEmployeeFilters `db:"-" json:"-"`
	CompanyName     string             `db:"-" json:"company_name"`
}

type CPEnrollment struct {
	ID                int64   `db:"id, primarykey, autoincrement" json:"id"`
	CompanyID         int64   `db:"company_id" json:"company_id"`
	EmployeeAccountID int64   `db:"employee_account_id" json:"employee_account_id"`
	Position          int8    `db:"position" json:"position"`
	CreatedDate       string  `db:"created_date" json:"created_date"`
	InstallationID    int64   `db:"installation_id" json:"installation_id"`
	Data              *string `db:"data" json:"data,omitempty"`
	Active            bool    `db:"active" json:"active"`
}

type CPEmployee struct {
	ID            int64           `db:"id, primarykey, autoincrement" json:"id"`
	CompanyID     int64           `db:"company_id" json:"company_id"`
	AccountID     int64           `db:"account_id" json:"account_id"`
	FirstName     string          `db:"first_name" json:"first_name"`
	MiddleInitial string          `db:"middle_initial" json:"middle_initial"`
	LastName      string          `db:"last_name" json:"last_name"`
	EmployeeID    string          `db:"employee_id" json:"employee_id"`
	Enrollments   *[]CPEnrollment `db:"-" json:"enrollments,omitempty"`
	Active        bool            `db:"active" json:"active"`

	CostCenter0 int64 `db:"cost_center_0" json:"cost_center_0"`
	CostCenter1 int64 `db:"cost_center_1" json:"cost_center_1"`
	CostCenter2 int64 `db:"cost_center_2" json:"cost_center_2"`
	CostCenter3 int64 `db:"cost_center_3" json:"cost_center_3"`
	CostCenter4 int64 `db:"cost_center_4" json:"cost_center_4"`
}

type NewEnrollmentRequest struct {
	InstallationToken string `json:"installation_token"`
	EmployeeAccountID int64  `json:"employee_account_id"`
	Data              string `json:"data"`
	Position          int8   `json:"position"`
	CreatedDate       string `json:"created_date"`
}

type NewEnrollmentResponse struct {
	GlobalID int64 `json:"global_id"`
}

type NewCPInstallationRequest struct {
	Name            string       `json:"name"`
	EmployeeFilters *PropertyMap `json:"employee_filters"`
}

type CPClientSettings struct {
}

type CPEmployeeFilters struct {
	CostCenters CPEmployeeCostCenterFilter `json:"cost_centers"`
}

type CPEmployeeCostCenterFilter struct {
	Indexes []CPEmployeeCostCenterFilterIndex `json:"indexes"`
}

type CPEmployeeCostCenterFilterIndex struct {
	Index  int `json:"index"`
	Values []struct {
		CostCenterID int64 `json:"cost_center_id"`
	} `json:"values"`
}

type DeleteEnrollmentRequest struct {
	InstallationToken string `json:"installation_token"`
	GlobalID          int64  `json:"global_id"`
}

type AlphaOrder []CPEmployee

func (a AlphaOrder) Len() int      { return len(a) }
func (a AlphaOrder) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a AlphaOrder) Less(i, j int) bool {
	if strings.ToUpper(a[i].FirstName) == strings.ToUpper(a[j].FirstName) {
		return strings.ToUpper(a[i].LastName) < strings.ToUpper(a[j].LastName)
	}
	return strings.ToUpper(a[i].FirstName) < strings.ToUpper(a[j].FirstName)
}

const (
	PRODUCT_CLOUDPUNCH_URL = "cloudpunch"
)

func registerCloudPunchRoutes(router *gin.Engine) {
	router.GET("/api/tlm/webclock", webClockGetHandler)
	router.POST("/api/tlm/webclock", webClockPostHandler)
	router.POST("/api/tlm/webclock/getname", webClockNamePostHandler)
	router.POST("/api/tlm/webclock/attestation", webClockAttestationPostHandler)
	router.POST("/api/tlm/punches", timeClockPunchHandler)
	router.GET("/api/tlm/cp/companyinfo", getAllCPDataHandler)
	router.GET("/api/tlm/cp/companyinfo/installations", getCPInstallationsHandler)
	router.POST("/api/tlm/cp/companyinfo/employee/:accountID/toggleActive", toggleCPEmployeeActiveHandler)
	router.GET("/api/tlm/cp/companyinfo/employees", getCPEmployeesHandler)
	router.GET("/api/tlm/cp/companyinfo/employees/search", searchCloudPunchEmployeesHandler)
	router.GET("/api/tlm/cp/installations", getAllCPInstallationsHandler)
	router.POST("/api/tlm/cp/installations", newCPInstallationHandler)
	router.POST("/api/tlm/cp/installations/:installationToken/update", updateCPInstallationHandler)
	router.POST("/api/tlm/cp/enrollments", newCPEnrollmentHandler)
	router.GET("/api/tlm/cp/sync", getCPSyncHandler)
	router.POST("/api/tlm/cp/sync/deleteenrollment", deleteCPEnrollmentHandler)
}

func toggleCPEmployeeActiveHandler(c *gin.Context) {
	thisCompany, err := lookupCompany(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	accountIDStr := c.Param("accountID")
	if accountIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	accountID, err := strconv.ParseInt(accountIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Employee not found"})
		return
	}

	cpEmp, err := getCPEmployeeByAccountID(accountID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Employee not found"})
		return
	}

	if thisCompany.ID != cpEmp.CompanyID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	cpEmp.Active = !cpEmp.Active
	dbmap.Update(cpEmp)

	if cpEmp.Active {
		go cpEmp.ProcessToggleActive(cpEmp.Active)
		c.JSON(http.StatusOK, cpEmp)
	} else {
		go cpEmp.ProcessToggleActive(cpEmp.Active)
		c.JSON(http.StatusOK, cpEmp)
	}
}

func getAllCPDataHandler(c *gin.Context) {
	thisCompany, err := lookupCompany(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	resp := CPAllDataResponse{}

	instlns, err := getAllCompanyCPInstallations(thisCompany)
	if err != nil {
		ErrorLog.Println("getAllCompanyInstallations err:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "An error occured"})
		return
	}

	resp.Installations = instlns

	empsResp := []CPEmployee{}

	emps, err := getAllCompanyCPEmployees(thisCompany, true, false)
	if err != nil {
		ErrorLog.Println("getAllCompanyCPEmployees err:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "An error occured"})
		return
	}

	for _, emp := range emps {
		empR := emp

		enrs, err := getCPEnrollmentsByEmployee(emp.AccountID, false, false)
		if err != nil {
			ErrorLog.Println("getCPEnrollmentsByEmployee err:", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "An error occured"})
			return
		}

		empR.Enrollments = &enrs

		empsResp = append(empsResp, empR)
	}

	resp.Employees = empsResp

	c.JSON(http.StatusOK, resp)
}

func getCPInstallationsHandler(c *gin.Context) {
	thisCompany, err := lookupCompany(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	instlns, err := getAllCompanyCPInstallations(thisCompany)
	if err != nil {
		ErrorLog.Println("getCPInstallationsHandler getAllCompanyInstallations err:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "An error occured"})
		return
	}

	c.JSON(http.StatusOK, instlns)
}

func getCPEmployeesHandler(c *gin.Context) {
	thisCompany, err := lookupCompany(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	emps, err := getAllCompanyCPEmployees(thisCompany, true, false)
	if err != nil {
		ErrorLog.Println("getAllCompanyCPEmployees err:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "An error occured"})
		return
	}

	empsResp := []CPEmployee{}

	for _, emp := range emps {
		empR := emp

		enrs, err := getCPEnrollmentsByEmployee(emp.AccountID, false, false)
		if err != nil {
			ErrorLog.Println("getCPEnrollmentsByEmployee err:", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "An error occured"})
			return
		}

		empR.Enrollments = &enrs

		empsResp = append(empsResp, empR)
	}

	c.JSON(http.StatusOK, empsResp)
}

func deleteCPEnrollmentHandler(c *gin.Context) {
	thisCompany, err := lookupCompany(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	input := &DeleteEnrollmentRequest{}
	if err := c.ShouldBindWith(input, binding.JSON); err != nil {
		ErrorLog.Println("err binding deleteCPEnrollmentHandler json: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	enr, err := getCPEnrollmentByID(input.GlobalID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Enrollment not found"})
		return
	}

	if enr.CompanyID != thisCompany.ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	err = enr.ToggleActive()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "An error occurred"})
		return
	}

	c.JSON(http.StatusOK, enr)

	go cpCreateEnrollmentDeletionChange(enr)
}

func newCPEnrollmentHandler(c *gin.Context) {
	thisCompany, err := lookupCompany(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	input := &NewEnrollmentRequest{}
	if err := c.ShouldBindWith(input, binding.JSON); err != nil {
		ErrorLog.Println("err binding newCPEnrollmentHandler json: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	_, err = insertCPEmployeeIfNotExists(input.EmployeeAccountID, &thisCompany)
	if err != nil {
		ErrorLog.Println("err insertCPEmployeeIfNotExists: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "An error occured"})
		return
	}

	err = deactivateExistingEnrollments(input.Position, input.EmployeeAccountID)
	if err != nil {
		ErrorLog.Println("err deactivateExistingEnrollments: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "An error occured"})
		return
	}

	enr, err := createCPEnrollment(input, &thisCompany)
	if err != nil {
		ErrorLog.Println("err createCPEnrollment: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "An error occured"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"global_id": enr.ID})

	go cpCreateNewAddEnrollmentChange(enr)
}

func getAllCPInstallationsHandler(c *gin.Context) {
	thisCompany, err := lookupCompany(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	ins, _ := getAllCompanyCPInstallations(thisCompany)

	c.JSON(http.StatusOK, ins)
}

func searchCloudPunchEmployeesHandler(c *gin.Context) {
	thisCompany, err := lookupCompany(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	fNameQueryLowered := strings.ToLower(c.Query("fname"))
	lNameQueryLowered := strings.ToLower(c.Query("lname"))

	records, err := getAllCompanyEmployeesWithCache(&thisCompany, false, true)
	if err != nil {
		ErrorLog.Println("searchCloudPunchEmployeesHandler err looking up companyID: ", thisCompany.OPID, " getAllCompanyEmployeesWithCache err: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Employee Not Found"})
		return
	}

	foundEmps := []CPEmployee{}
	for _, emp := range *records {
		if strings.ToLower(emp.FirstName) == fNameQueryLowered {
			if strings.ToLower(emp.LastName) == lNameQueryLowered {
				cpe, err := getCPEmployeeByAccountID(int64(emp.ID))
				if err != nil {
					fullEmp, err := getFullEmployeeInfoWithCache(&thisCompany, int64(emp.ID))
					if err != nil {
						ErrorLog.Println("getFullEmployeeInfoWithCache err: ", err)
						continue
					}

					cpEmpDoesntExist := CPEmployee{
						CompanyID:     thisCompany.ID,
						AccountID:     int64(fullEmp.ID),
						FirstName:     fullEmp.FirstName,
						MiddleInitial: opMiddleNameToMiddleInitial(fullEmp.MiddleName),
						LastName:      fullEmp.LastName,
						EmployeeID:    fullEmp.EmployeeID,
					}

					foundEmps = append(foundEmps, cpEmpDoesntExist)
				} else {
					enrs, err := getCPEnrollmentsByEmployee(int64(emp.ID), true, true)
					if err == nil {
						if len(enrs) > 0 {
							cpe.Enrollments = &enrs
						}
					}
					foundEmps = append(foundEmps, *cpe)
				}
			}
		}
	}

	if foundEmps != nil {
		c.JSON(http.StatusOK, foundEmps)
		return
	}

	c.JSON(http.StatusBadRequest, gin.H{"error": "Employee Not Found"})
	return
}

func newCPInstallationHandler(c *gin.Context) {
	thisCompany, err := lookupCompany(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	input := &NewCPInstallationRequest{}
	if err := c.ShouldBindWith(input, binding.JSON); err != nil {
		ErrorLog.Println("err binding newCPInstallationHandler json: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	clientIP := c.GetHeader("X-Real-IP")

	newIns, err := newCPInstallation(thisCompany, clientIP, input.Name, input.EmployeeFilters)
	if err != nil {
		ErrorLog.Println("newCPInstallation err: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "An error occured"})
		return
	}

	c.JSON(http.StatusCreated, newIns)
}

func updateCPInstallationHandler(c *gin.Context) {
	thisCompany, err := lookupCompany(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	token := c.Param("installationToken")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	clientIP := c.GetHeader("X-Real-IP")

	input := struct {
		NewName         *string      `json:"display_name"`
		EmployeeFilters *PropertyMap `json:"employee_filters,omitempty"`
	}{}
	if err := c.ShouldBindWith(&input, binding.JSON); err != nil {
		ErrorLog.Println("err binding newCPInstallationHandler json: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	install, err := getInstallationByToken(token)
	if err != nil {
		ErrorLog.Println("getInstallationByToken err: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Installation does not exist"})
		return
	}

	if install.CompanyID != thisCompany.ID {
		ErrorLog.Println("getInstallationByToken install.CompanyID != thisCompany.ID ")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	if input.NewName != nil && *input.NewName != "" {
		install.DisplayName = *input.NewName
	}

	employeeFilterUpdated := false
	if input.EmployeeFilters != nil {
		cef := input.EmployeeFilters.ToCPEmployeeFilters()
		if cef != nil && cef.CostCenters.Indexes != nil {
			install.EmployeeFiltersPM = cef.ToPropertyMap()
			employeeFilterUpdated = true
		}
	}

	install.IP = clientIP
	dbmap.Update(&install)

	c.JSON(http.StatusOK, install)

	if employeeFilterUpdated {
		go install.HandleEmployeeFiltersUpdate()
	}
}

func (i *CPInstallation) HandleEmployeeFiltersUpdate() {
	data := CPUpdateFiltersDB{
		InstallationID: i.ID,
	}

	var dataPM PropertyMap
	datab, err := json.Marshal(data)
	if err != nil {
		return
	}
	err = json.Unmarshal(datab, &dataPM)
	if err != nil {
		return
	}

	newChange := &CPChange{
		CompanyID: i.CompanyID,
		Type:      CPCHANGETYPE_UPDATE_INSTALL_FILTER,
		Data:      &dataPM,
	}

	err = dbmap.Insert(newChange)

	return
}

func newCPInstallation(company Company, ipAddress, name string, employeeFilters *PropertyMap) (*CPInstallation, error) {
	newToken, err := generateNewToken()
	if err != nil {
		return nil, err
	}

	nameToUse := name
	if name == "" {
		existingInstalls, err := getAllCompanyCPInstallations(company)
		if err != nil {
			return nil, err
		}

		nameToUse = fmt.Sprintf("CloudPunch Installation #%d", len(existingInstalls)+1)
	}

	cef := employeeFilters.ToCPEmployeeFilters()
	if cef == nil || cef.CostCenters.Indexes == nil {
		defaultFilters := CPEmployeeFilters{}
		x := CPEmployeeCostCenterFilter{}
		x.Indexes = []CPEmployeeCostCenterFilterIndex{}
		defaultFilters.CostCenters = x

		employeeFilters = defaultFilters.ToPropertyMap()
	} else {
		employeeFilters = cef.ToPropertyMap()
	}

	cpi := &CPInstallation{
		Token:             newToken,
		CompanyID:         company.ID,
		DisplayName:       nameToUse,
		IP:                ipAddress,
		EmployeeFiltersPM: employeeFilters,
		ClientSettingsPM:  &PropertyMap{},
		CompanyName:       company.Name,
	}

	err = dbmap.Insert(cpi)
	if err != nil {
		return nil, err
	}

	return cpi, nil
}

// Relays OnePoint API
// If no errors with server, will always return 200
// Even if 200, could still have errors per punch
// If server error, will return {error: "error statement"}
func timeClockPunchHandler(c *gin.Context) {
	thisCompany, err := lookupCompany(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	input := opapi.TimePunchRequest{}
	if err := c.ShouldBindWith(&input, binding.JSON); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	cxn := chooseOPAPICxn(thisCompany.OPID)

	const logTimeFmt = "01/02 15:04:05.000"

	InfoLog.Printf("[%s] SENDING PUNCH %s | %v / %v calls prior | client punch body: %+v\n", thisCompany.ShortName, time.Now().Format(logTimeFmt), cxn.CallLimit.CurrentCalls, cxn.CallLimit.Threshold, input)

	_, opPunchResponse, err, opError := cxn.SendTimePunches(thisCompany.OPID, &input)
	if err != nil {
		if opError != nil {
			ErrorLog.Printf("PUNCH ERR %s | %v / %v calls after | opError: %+v\n", time.Now().Format(logTimeFmt), cxn.CallLimit.CurrentCalls, cxn.CallLimit.Threshold, opError.Errors)
			c.JSON(http.StatusBadRequest, gin.H{"error": opError.String()})
			return
		} else {
			ErrorLog.Printf("PUNCH ERR %s | %v / %v calls after | INTERNAL err: %s\n", time.Now().Format(logTimeFmt), cxn.CallLimit.CurrentCalls, cxn.CallLimit.Threshold, err.Error())
			c.JSON(http.StatusBadRequest, gin.H{"error": "An error occurred"})
			return
		}
	}

	InfoLog.Printf("[%s] SENT PUNCH %s | %v / %v calls after | op response body: %+v\n", thisCompany.ShortName, time.Now().Format(logTimeFmt), cxn.CallLimit.CurrentCalls, cxn.CallLimit.Threshold, opPunchResponse)

	c.JSON(http.StatusOK, opPunchResponse)
}

func refreshTimeClockProductCaches() {
	InfoLog.Println("starting refreshCloudPunchCaches")

	timeClockProducts := []string{PRODUCT_CLOUDPUNCH_URL, PRODUCT_WEBCLOCK_URL}

	allCPCompanies := []CompanyProduct{}
	for _, product := range timeClockProducts {
		productCompanies, err := findAllCompaniesWithProduct(product)
		if err != nil {
			ErrorLog.Println("findAllCompaniesWithProduct err: ", err.Error())
			return
		}
		allCPCompanies = append(allCPCompanies, productCompanies...)
	}

	alreadyRefreshedCompanies := make(map[int64]bool)
	for i, cp := range allCPCompanies {
		if _, alreadyPassed := alreadyRefreshedCompanies[cp.CompanyID]; alreadyPassed {
			continue
		}
		company, _ := lookupCompanyByID(cp.CompanyID)
		cxn := chooseOPAPICxn(company.OPID)

		_, err := getAllCostCentersWithCache(cxn, &company, true)
		if err != nil {
			ErrorLog.Println(company.ShortName, " getAllCostCentersWithCache err: ", err.Error())
		}

		_, err = getAllCompanyEmployeesWithCache(&company, true, true)
		if err != nil {
			ErrorLog.Println(company.ShortName, " getAllCompanyEmployeesWithCache err: ", err.Error())
		}

		alreadyRefreshedCompanies[cp.CompanyID] = true

		// sleep for rate limits
		if i != (len(allCPCompanies) - 1) {
			time.Sleep(30 * time.Second)
		}
	}

	InfoLog.Println("finished refreshCloudPunchCaches")
}

func getAllCompanyCPInstallations(company Company) ([]CPInstallation, error) {
	cpis := []CPInstallation{}
	_, err := dbmap.Select(&cpis, "SELECT * FROM cp_installations WHERE company_id = ?", company.ID)
	return cpis, err
}

func getAllCompanyCPEmployees(company Company, alphaOrder bool, activeOnly bool) ([]CPEmployee, error) {
	cpis := []CPEmployee{}
	qry := "SELECT * FROM cp_employees WHERE company_id = ?"
	if activeOnly {
		qry = "SELECT * FROM cp_employees WHERE company_id = ? AND active = 1"
	}
	_, err := dbmap.Select(&cpis, qry, company.ID)
	if err == nil {
		sort.Sort(AlphaOrder(cpis))
	}
	return cpis, err
}

func getInstallationByToken(token string) (CPInstallation, error) {
	cpi := CPInstallation{}
	err := dbmap.SelectOne(&cpi, "SELECT * FROM cp_installations WHERE token = ?", token)
	if err != nil {
		return cpi, err
	}
	cpi.ConvertPropMaps()
	return cpi, err
}

func (c *CPInstallation) ConvertPropMaps() {
	var result CPEmployeeFilters
	datab, err := json.Marshal(c.EmployeeFiltersPM)
	if err != nil {
		return
	}
	err = json.Unmarshal(datab, &result)
	if err != nil {
		return
	}

	c.EmployeeFilters = &result
}

func getCPEmployeeByAccountID(accID int64) (*CPEmployee, error) {
	cpe := &CPEmployee{}
	err := dbmap.SelectOne(&cpe, "SELECT * FROM cp_employees WHERE account_id = ?", accID)
	return cpe, err
}

func insertCPEmployeeIfNotExists(accID int64, company *Company) (*CPEmployee, error) {
	existingEmp, err := getCPEmployeeByAccountID(accID)
	if err != nil {
		newEmp, err := createCPEmployee(accID, company)
		return newEmp, err
	} else {
		return existingEmp, nil
	}
}

func opMiddleNameToMiddleInitial(mn string) string {
	mi := ""
	if mn != "" {
		mi = strings.ToUpper(string([]rune(mn)[0]))
	}
	return mi
}

func createCPEmployee(accID int64, company *Company) (*CPEmployee, error) {
	cxn := chooseOPAPICxn(company.OPID)
	empRecord, err := cxn.GetFullEmployeeInfo(accID, company.OPID)
	if err != nil {
		return nil, err
	}

	newEmp := &CPEmployee{
		CompanyID:     company.ID,
		AccountID:     accID,
		FirstName:     empRecord.FirstName,
		MiddleInitial: opMiddleNameToMiddleInitial(empRecord.MiddleName),
		LastName:      empRecord.LastName,
		EmployeeID:    empRecord.EmployeeID,
		Active:        true,
	}

	for _, cc := range empRecord.CostCentersInfo.Defaults {
		switch cc.Index {
		case 0:
			newEmp.CostCenter0 = int64(cc.Value.ID)
		case 1:
			newEmp.CostCenter1 = int64(cc.Value.ID)
		case 2:
			newEmp.CostCenter2 = int64(cc.Value.ID)
		case 3:
			newEmp.CostCenter3 = int64(cc.Value.ID)
		case 4:
			newEmp.CostCenter4 = int64(cc.Value.ID)
		}
	}

	err = dbmap.Insert(newEmp)

	return newEmp, err
}

func createCPEnrollment(enrReq *NewEnrollmentRequest, company *Company) (*CPEnrollment, error) {
	inst, err := getInstallationByToken(enrReq.InstallationToken)
	if err != nil {
		return nil, err
	}

	newEnr := &CPEnrollment{
		CompanyID:         company.ID,
		EmployeeAccountID: enrReq.EmployeeAccountID,
		Position:          enrReq.Position,
		CreatedDate:       enrReq.CreatedDate,
		InstallationID:    inst.ID,
		Data:              &enrReq.Data,
		Active:            true,
	}
	err = dbmap.Insert(newEnr)

	return newEnr, err
}

func deactivateExistingEnrollments(fingerPostion int8, accID int64) error {
	existing := []CPEnrollment{}
	_, err := dbmap.Select(&existing, "SELECT * FROM cp_enrollments WHERE active = 1 AND employee_account_id = ? AND position = ?", accID, fingerPostion)
	if err != nil {
		return err
	}

	for _, enr := range existing {
		deactivateEnrollment(&enr)
	}

	return nil
}

func deactivateEnrollment(enr *CPEnrollment) error {
	enr.Active = false
	_, err := dbmap.Update(enr)
	return err
}

func getCPEnrollmentByID(enrID int64) (*CPEnrollment, error) {
	cpe := &CPEnrollment{}
	err := dbmap.SelectOne(&cpe, "SELECT * FROM cp_enrollments WHERE id = ?", enrID)
	return cpe, err
}

func (enr *CPEnrollment) ToggleActive() error {
	enr.Active = !enr.Active
	_, err := dbmap.Update(enr)
	return err
}

func (i *CPInstallation) UpdateLastSyncedAndIP(t time.Time, newIP string) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err == nil {
		t = t.In(loc)
	}

	i.IP = newIP
	i.LastSynced = t.Format("15:04:05 MST Jan 2, 2006")
	dbmap.Update(i)
}

func getCPEnrollmentsByEmployee(empAccountID int64, includeData, activeOnly bool) ([]CPEnrollment, error) {
	enrs := []CPEnrollment{}
	qry := "SELECT * FROM cp_enrollments WHERE employee_account_id = ?"
	if !includeData {
		qry = "SELECT id, company_id, employee_account_id, position, created_date, installation_id, active FROM cp_enrollments WHERE employee_account_id = ?"
	}

	if activeOnly {
		qry = qry + " AND active = 1"
	}

	_, err := dbmap.Select(&enrs, qry, empAccountID)

	return enrs, err
}
