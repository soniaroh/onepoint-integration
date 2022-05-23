package main

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

type AccessControl struct {
	ID               int64  `db:"id, primarykey, autoincrement" json:"id"`
	CompanyID        int64  `db:"company_id" json:"company_id"`
	CompanyProductID int64  `db:"company_product_id" json:"company_product_id"`
	EmployeeOPID     int64  `db:"employee_op_id" json:"employee_op_id"`
	EmployeeName     string `db:"employee_name" json:"employee_name"`
	AccessType       string `db:"access_type" json:"access_type"`
	Active           bool   `db:"active"  json:"active"`
}

type Authorization struct {
	Read  bool `json:"read"`
	Write bool `json:"write"`
}

func checkUserAuthorized(user *User, company *Company, productURL string, accessType string) bool {
	if user.IsSystemAdmin {
		return true
	}

	if user.IsCompanyAdmin {
		if company.ID == user.CompanyID {
			return true
		}
	}

	ac := &AccessControl{}
	err := dbmap.SelectOne(ac, `
		SELECT ac.*
		FROM access_controls ac, company_products cps, products p
		WHERE p.url = ? AND p.id = cps.product_id AND ac.company_product_id = cps.id
			AND ac.employee_op_id = ?
		`, productURL, user.OPID)

	if err != nil {
		ErrorLog.Println("checkUserAuthorized err: ", err)
		return false
	}

	if ac.Active {
		if ac.AccessType == "W" || ac.AccessType == accessType {
			return true
		}
	}

	return true
}

func createAccessControlHandler(c *gin.Context) {
	thisUser, err := lookupThisUser(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	if !(thisUser.IsCompanyAdmin || thisUser.IsSystemAdmin) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	thisCompany, err := lookupCompany(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	input := &AccessControl{}
	if err := c.ShouldBindWith(&input, binding.JSON); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	input.Active = true

	_, err = createNewAccessControl(thisUser, &thisCompany, input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// return all because of redux shortcut, need to eventually return only this one
	cps, err := getCompanysProducts(thisUser, thisCompany.ID)
	if err != nil {
		ErrorLog.Println("err getCompanysProducts: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "An error occured"})
		return
	}

	c.JSON(http.StatusOK, cps)
}

func editAccessControlHandler(c *gin.Context) {
	companyProductAccessIdStr := c.Param("companyProductAccessId")
	companyProductAccessId, err := strconv.ParseInt(companyProductAccessIdStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "The id must be an integer"})
		return
	}

	thisUser, err := lookupThisUser(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	if !(thisUser.IsCompanyAdmin || thisUser.IsSystemAdmin) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	thisCompany, err := lookupCompany(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	input := &AccessControl{}
	if err := c.ShouldBindWith(&input, binding.JSON); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	_, err = editAccessControl(companyProductAccessId, thisUser, &thisCompany, input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// return all because of redux shortcut, need to eventually return only this one
	cps, err := getCompanysProducts(thisUser, thisCompany.ID)
	if err != nil {
		ErrorLog.Println("err getCompanysProducts: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "An error occured"})
		return
	}

	c.JSON(http.StatusOK, cps)
}

func editAccessControl(acID int64, user *User, company *Company, ac *AccessControl) (*AccessControl, error) {
	ac.ID = acID

	existingAC := &AccessControl{}
	err := dbmap.SelectOne(existingAC, "SELECT * FROM access_controls WHERE id = ?", acID)
	if err != nil {
		return nil, errors.New("That Access Control does not exist")
	}

	// cannot update these fields
	ac.EmployeeName = existingAC.EmployeeName
	ac.EmployeeOPID = existingAC.EmployeeOPID

	_, err = validateAccessControl(user, company, ac)
	if err != nil {
		return nil, errors.New(err.Error())
	}

	_, err = dbmap.Update(ac)
	if err != nil {
		ErrorLog.Println("editAccessControl err Update:", err)
		return nil, errors.New("Could not update the Access Control")
	}

	return ac, nil
}

func createNewAccessControl(user *User, company *Company, ac *AccessControl) (*AccessControl, error) {
	_, err := validateAccessControl(user, company, ac)
	if err != nil {
		return nil, errors.New(err.Error())
	}

	// verfiy this does not already exist
	existing := []AccessControl{}
	_, err = dbmap.Select(&existing, "SELECT * FROM access_controls WHERE employee_op_id = ? AND company_product_id = ?", ac.EmployeeOPID, ac.CompanyProductID)
	if err != nil {
		ErrorLog.Println("createNewAccessControl Select err: ", err)
		return nil, errors.New("An error occured while creating the Access Control")
	}

	if len(existing) > 0 {
		ErrorLog.Println("createNewAccessControl Select err: ", err)
		return nil, errors.New("That user/product Access Control pair already exists")
	}

	err = dbmap.Insert(ac)
	if err != nil {
		ErrorLog.Println("createNewAccessControl Insert err: ", err)
		return nil, errors.New("An error occured while creating the Access Control")
	}

	return ac, nil
}

func validateAccessControl(user *User, company *Company, ac *AccessControl) (*AccessControl, error) {
	if !(user.IsCompanyAdmin || user.IsSystemAdmin) {
		return nil, errors.New("Not authorized")
	}

	ac.CompanyID = company.ID

	switch ac.AccessType {
	case "R":
		break
	case "W":
		break
	default:
		return nil, errors.New("Access type is wrong format")
	}

	if ac.EmployeeName == "" {
		return nil, errors.New("Employee name cannot be empty")
	}

	// make sure company prod is valid
	cp, err := getCompanyProductById(ac.CompanyProductID)
	if err != nil {
		ErrorLog.Println("createNewAccessControl CompanyProductID does not exist")
		return nil, errors.New("Invalid company product ID")
	}

	if cp.CompanyID != company.ID {
		ErrorLog.Println("createNewAccessControl cp.CompanyID != company.ID")
		return nil, errors.New("Not authorized")
	}

	return ac, nil
}

func getCompanyProductsAccessControls(activeOnly bool, companyProductID int64) ([]AccessControl, error) {
	acs := []AccessControl{}
	qry := "SELECT * FROM access_controls WHERE company_product_id = ?"
	if activeOnly {
		qry = "SELECT * FROM access_controls WHERE company_product_id = ? AND active = 1"
	}
	_, err := dbmap.Select(&acs, qry, companyProductID)
	return acs, err
}
