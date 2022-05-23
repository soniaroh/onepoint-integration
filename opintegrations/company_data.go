package main

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"opapi"
	"github.com/gin-gonic/gin"
)

type ByFirstName []EmployeePublic

func (a ByFirstName) Len() int      { return len(a) }
func (a ByFirstName) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByFirstName) Less(i, j int) bool {
	return strings.ToUpper(a[i].Name) < strings.ToUpper(a[j].Name)
}

type EmployeePublic struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
	ID    int    `json:"id"`
}

func registerCompanyRoutes(router *gin.Engine) {
	router.POST("/api/companies", addCompany)
	router.GET("/api/companydata/employees/searchByName", searchEmployeesHandler)
	router.GET("/api/company/:dataName", fetchCompanyDataHandler)
}

func getAllCompanyEmployeesWithCache(company *Company, forceRefresh, activeOnly bool) (*[]opapi.OPFullEmployeeRecord, error) {
	cacheKey := fmt.Sprintf("%s%s", company.ShortName, CACHENAME_COMPANY_EMPLOYEES)

	if !forceRefresh {
		cachedEmployeesInterface, found := cash.Get(cacheKey)
		if found {
			cachedEmployees, isType := cachedEmployeesInterface.(*[]opapi.OPFullEmployeeRecord)
			if isType {
				InfoLog.Println(company.ShortName, " getAllCompanyEmployeesWithCache hit cache!")

				if cachedEmployees != nil {
					empsWeWantToSave := filterEmpsWeWant(*cachedEmployees, true, activeOnly)
					return &empsWeWantToSave, nil
				}

				return nil, errors.New("cachedEmployees was nil")
			}
		}
	}

	InfoLog.Printf("%s getAllCompanyEmployeesWithCache did NOT hit cache, with forceRefresh = %t\n", company.ShortName, forceRefresh)

	cxn := chooseOPAPICxn(company.OPID)

	records, err := cxn.GetAllCompanyEmployees(company.OPID)
	if err != nil {
		return nil, err
	}

	empsWeWantToSave := filterEmpsWeWant(records, true, activeOnly)

	cash.Set(cacheKey, &empsWeWantToSave, 50*time.Minute)

	return &empsWeWantToSave, nil
}

func filterEmpsWeWant(emps []opapi.OPFullEmployeeRecord, removeLinks, activeOnly bool) []opapi.OPFullEmployeeRecord {
	empsWeWantToSave := []opapi.OPFullEmployeeRecord{}
	for _, emp := range emps {
		// remove the links to store less data
		if removeLinks {
			emp.Links = nil
		}

		if activeOnly {
			if emp.Dates.Terminated != "" {
				continue
			}
		}

		empsWeWantToSave = append(empsWeWantToSave, emp)
	}

	return empsWeWantToSave
}

func getFullEmployeeInfoWithCache(company *Company, employeeOPID int64) (*opapi.OPFullEmployeeRecord, error) {
	cacheKey := fmt.Sprintf("%d%s", employeeOPID, CACHENAME_EMPLOYEE_DETAIL)

	cachedEmployeeInterface, found := cash.Get(cacheKey)
	if found {
		cachedEmployee, isType := cachedEmployeeInterface.(*opapi.OPFullEmployeeRecord)
		if isType {
			InfoLog.Println(company.ShortName, " emp:", employeeOPID, ", getFullEmployeeInfoWithCache hit cache!")

			return cachedEmployee, nil
		}
	}

	InfoLog.Println(company.ShortName, " emp:", employeeOPID, ", getFullEmployeeInfoWithCache DID NOT hit cache!")

	cxn := chooseOPAPICxn(company.OPID)

	fullEmp, err := cxn.GetFullEmployeeInfo(employeeOPID, company.OPID)
	if err != nil {
		return nil, err
	}

	cash.Set(cacheKey, &fullEmp, 50*time.Minute)

	return &fullEmp, nil
}

// deprecated in favor of a multi result search
func searchEmployeesHandler(c *gin.Context) {
	thisCompany, err := lookupCompany(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	fNameQueryLowered := strings.ToLower(c.Query("fname"))
	lNameQueryLowered := strings.ToLower(c.Query("lname"))

	records, err := getAllCompanyEmployeesWithCache(&thisCompany, false, true)
	if err != nil {
		ErrorLog.Println("searchEmployeesHandler err looking up companyID: ", thisCompany.OPID, " getAllCompanyEmployeesWithCache err: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Employee Not Found"})
		return
	}

	var foundEmp *opapi.OPFullEmployeeRecord
	for _, emp := range *records {
		if strings.ToLower(emp.FirstName) == fNameQueryLowered {
			if strings.ToLower(emp.LastName) == lNameQueryLowered {
				foundEmp = &emp
				break
			}
		}
	}

	if foundEmp != nil {
		fullEmp, err := getFullEmployeeInfoWithCache(&thisCompany, int64(foundEmp.ID))
		if err != nil {
			ErrorLog.Println("searchEmployeesHandler GetFullEmployeeInfo err looking up companyID: ", thisCompany.OPID, " GetFullEmployeeInfo: ", foundEmp.ID)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Employee Not Found"})
			return
		}

		c.JSON(http.StatusOK, fullEmp)
		return
	}

	c.JSON(http.StatusBadRequest, gin.H{"error": "Employee Not Found"})
	return
}

func getAllCostCentersWithCache(cxn *opapi.OPConnection, company *Company, forceRefresh bool) (*[]opapi.CostCenterTree, error) {
	cacheKey := fmt.Sprintf("%s%s", company.ShortName, CACHENAME_COMPANY_COST_CENTERS)

	cachedCCsInterface, found := cash.Get(cacheKey)
	if !forceRefresh && found {
		cachedCCs, isType := cachedCCsInterface.(*[]opapi.CostCenterTree)
		if isType {
			InfoLog.Println(company.ShortName, " getAllCostCentersWithCache hit cache!")

			return cachedCCs, nil
		}
	}

	InfoLog.Printf("%s getAllCostCentersWithCache did NOT hit cache, with forceRefresh = %t\n", company.ShortName, forceRefresh)

	records, err := cxn.GetAllCompanyCostCenters(company.OPID, true)
	if err != nil {
		return &records, err
	}

	cash.Set(cacheKey, &records, 40*time.Minute)

	return &records, nil
}

func fetchCompanyDataHandler(c *gin.Context) {
	thisCompany, err := lookupCompany(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	dataName := c.Param("dataName")
	if dataName == "" {
		ErrorLog.Println("blank dataName path parameter")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	InfoLog.Printf("data fetch company[%s] data[%s]\n", thisCompany.ShortName, dataName)

	cxn := chooseOPAPICxn(thisCompany.OPID)

	switch dataName {
	case "employees":
		records, err := getAllCompanyEmployeesWithCache(&thisCompany, false, true)
		if err != nil {
			ErrorLog.Println("err looking up companyID: ", thisCompany.OPID, " getAllCompanyEmployeesWithCache: ", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Not Found"})
			return
		}

		if records == nil {
			ErrorLog.Println("err looking up companyID: ", thisCompany.OPID, " getAllCompanyEmployeesWithCache: ", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Not Found"})
			return
		}

		data := []EmployeePublic{}
		for _, record := range *records {
			emp := EmployeePublic{
				Name: fmt.Sprintf("%s %s", record.FirstName, record.LastName),
				ID:   record.ID,
			}

			data = append(data, emp)
		}

		sort.Sort(ByFirstName(data))

		c.JSON(200, gin.H{"data": gin.H{"name": "employees", "values": data}})
	case "cost-centers":
		forceRefresh := c.Query("forceRefresh")
		forceRefreshB := false
		if forceRefresh == "true" {
			forceRefreshB = true
		}

		records, err := getAllCostCentersWithCache(cxn, &thisCompany, forceRefreshB)
		if err != nil {
			ErrorLog.Println("err looking up companyID: ", thisCompany.OPID, " GetAllCompanyCostCenters: ", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Not Found"})
			return
		}

		c.JSON(200, gin.H{"data": gin.H{"name": "costCenters", "values": records}})
	case "job-titles":
		records, err := cxn.GetOPJobs(thisCompany.OPID)
		if err != nil {
			ErrorLog.Println("err looking up companyID: ", thisCompany.OPID, " GetOPJobs: ", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Not Found"})
			return
		}

		c.JSON(200, gin.H{"data": gin.H{"name": "jobTitles", "values": records}})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not Found"})
		return
	}
}
