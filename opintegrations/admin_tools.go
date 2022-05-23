package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

func registerAdminRoutes(router *gin.Engine) {
	router.GET("/api/admin/startprocess/:processName", startAdminProcessHandler)
	router.POST("/api/admin/users", adminAddUserHandler)
	router.POST("/api/cacherefresh/:productName", refreshHandler)
}

func refreshHandler(c *gin.Context) {
	company, err := lookupCompany(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	cxn := chooseOPAPICxn(company.OPID)

	_, err = getAllCostCentersWithCache(cxn, &company, true)
	if err != nil {
		ErrorLog.Println(company.ShortName, " getAllCostCentersWithCache err: ", err.Error())
	}

	_, err = getAllCompanyEmployeesWithCache(&company, true, true)
	if err != nil {
		ErrorLog.Println(company.ShortName, " getAllCompanyEmployeesWithCache err: ", err.Error())
	}

	c.JSON(http.StatusOK, gin.H{"msg": "Success"})
	return
}

func adminAddUserHandler(c *gin.Context) {
	err := isAdminUser(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	input := CreateUserRequest{}
	if err := c.ShouldBindWith(&input, binding.JSON); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	company, err := lookupCompanyByID(input.CompanyID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Company not found"})
		return
	}

	newUser, err := processOPUserLoginV2(input.Username, &company, input.IsSystemAdmin, input.IsCompanyAdmin)
	if err != nil {
		ErrorLog.Println("processOPUserLoginV2 err: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "An error occured"})
		return
	}

	c.JSON(http.StatusCreated, newUser)
}

func startAdminProcessHandler(c *gin.Context) {
	err := isAdminUser(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	processName := c.Param("processName")
	if processName == "" {
		ErrorLog.Println("blank processName path parameter")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	switch processName {
	case "runReliasFeeds":
		go runAllReliasFeeds()
	case "runUserProvisioningChanges":
		go runUserProvisioningChanges()
	}

	c.JSON(200, gin.H{"message": "Process started"})
}
