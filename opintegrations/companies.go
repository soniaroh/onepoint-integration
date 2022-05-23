package main

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	uuid "github.com/satori/go.uuid"
)

type Company struct {
	ID        int64  `db:"id, primarykey, autoincrement"`
	OPID      int64  `db:"op_id" json:"op_id"`
	ShortName string `db:"short_name" json:"short_name"`
	Name      string `db:"name" json:"name"`
	APIKey    string `db:"api_key" json:"api_key"`
	Token     string `db:"token" json:"-"`
}

type AlphaByName []Company

func (a AlphaByName) Len() int      { return len(a) }
func (a AlphaByName) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a AlphaByName) Less(i, j int) bool {
	return strings.ToUpper(a[i].Name) < strings.ToUpper(a[j].Name)
}

type CompanyPublic struct {
	OPID      int64  `db:"op_id" json:"op_id"`
	ShortName string `db:"short_name" json:"short_name"`
	Name      string `db:"name" json:"name"`
}

func addCompany(c *gin.Context) {
	err := isAdminUser(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	input := Company{}
	if err := c.ShouldBindWith(&input, binding.JSON); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	if input.OPID == 0 || input.APIKey == "" || input.Name == "" || input.ShortName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing input"})
		return
	}

	existingCompany := &Company{}
	err = dbmap.SelectOne(existingCompany, "SELECT * FROM companies WHERE short_name = ?", input.ShortName)
	if err == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Company Short already exists"})
		return
	}

	newToken, err := generateNewToken()
	if err != nil {
		ErrorLog.Println("token gen err: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Something went wrong"})
		return
	}

	newCompany := &Company{
		OPID:      input.OPID,
		ShortName: input.ShortName,
		Name:      input.Name,
		APIKey:    input.APIKey,
		Token:     newToken,
	}

	err = dbmap.Insert(newCompany)
	if err != nil {
		ErrorLog.Println(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Something went wrong"})
		return
	}

	c.JSON(http.StatusCreated, newCompany)
}

func generateNewToken() (string, error) {
	tok, err := uuid.NewV4()
	if err != nil {
		return "", err
	}
	return tok.String(), nil
}

func getCompaniesMap(productID int64) map[int64]int64 {
	allCompaniesEnrolledInThisProduct := []Company{}
	_, err := dbmap.Select(&allCompaniesEnrolledInThisProduct, "SELECT c.* FROM companies c, company_products cps WHERE c.id=cps.company_id AND cps.product_id=? AND enabled=1", productID)
	if err != nil {
		ErrorLog.Println(err)
		return nil
	}

	m := map[int64]int64{}
	for _, company := range allCompaniesEnrolledInThisProduct {
		m[company.OPID] = company.ID
	}

	return m
}

func getOPCompanyID(internalID int64) (int64, error) {
	thisCompany := Company{}
	err := dbmap.SelectOne(&thisCompany, "SELECT * FROM companies WHERE id = ?", internalID)
	if err != nil {
		ErrorLog.Println("err looking up compnay by id: ", err)
		return 0, err
	}

	return thisCompany.OPID, nil
}

func lookupCompanyByOPID(opCompanyID int64) (Company, error) {
	thisCompany := Company{}
	err := dbmap.SelectOne(&thisCompany, "SELECT * FROM companies WHERE op_id = ?", opCompanyID)
	if err != nil {
		ErrorLog.Println("err looking up compnay by op_id: ", err)
		return thisCompany, err
	}

	return thisCompany, nil
}

func lookupCompanyByShortname(shortname string) (Company, error) {
	thisCompany := Company{}
	err := dbmap.SelectOne(&thisCompany, "SELECT * FROM companies WHERE short_name = ?", shortname)
	return thisCompany, err
}

func lookupCompanyByID(id int64) (Company, error) {
	thisCompany := Company{}
	err := dbmap.SelectOne(&thisCompany, "SELECT * FROM companies WHERE id = ?", id)
	return thisCompany, err
}
