package main

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

type CompanyProduct struct {
	ID        int64       `db:"id, primarykey, autoincrement" json:"id"`
	ProductID int64       `db:"product_id" json:"product_id"`
	CompanyID int64       `db:"company_id" json:"company_id"`
	Enabled   *bool       `db:"enabled" json:"enabled"`
	Settings  PropertyMap `db:"settings,size:1024" json:"settings"`
}

type CompanyProductAssignment struct {
	ID                   int64 `db:"id, primarykey, autoincrement" json:"id"`
	CompanyProductID     int64 `db:"company_product_id" json:"company_product_id"`
	CompanyIntegrationID int64 `db:"company_integration_id" json:"company_integration_id"`
}

type CompanyProductJoin struct {
	CompanyProductID int64                      `db:"company_product_id" json:"company_product_id"`
	Name             string                     `db:"name" json:"product_name"`
	Description      string                     `db:"description" json:"product_description"`
	URL              string                     `db:"url" json:"product_url"`
	Integrations     []CompanyIntegrationPublic `db:"-" json:"integrations"`
	Settings         PropertyMap                `json:"settings"`
	AccessControls   *[]AccessControl           `db:"-" json:"access_controls,omitempty"`
	Authorization    *Authorization             `db:"-" json:"authorization,omitempty"`
	Enabled          *bool                      `db:"enabled" json:"enabled"`
}

type CompanyProductsResponse struct {
	Products    []CompanyProductJoin `json:"products"`
	ThisCompany CompanyPublic        `json:"company"`
	ThisUser    *User                `json:"user,omitempty"`
}

type CompanyProductsPostRequest struct {
	CompanyID    int64        `json:"company_id"`
	ProductURL   string       `json:"product_url"`
	Settings     *PropertyMap `json:"settings"`
	Integrations []string     `json:"integrations"`
}

type CompanyProductListUpdateRequest struct {
	ChangeType string `json:"change_type"`
	ProductURL string `json:"product_url"`
}

func registerCompanyProductRoutes(router *gin.Engine) {
	router.POST("/api/companyproduct", addCompanyProductHandler)
	// this is bad url naming, variable needs to be at end
	router.POST("/api/companyproduct/:companyProductId/settings/update", updateCompanyProductHandler)
	router.POST("/api/companyproductlist/update", updateCompanysProductListHandler)
	router.POST("/api/companyproductaccess/new", createAccessControlHandler)
	router.POST("/api/companyproductaccess/edit/:companyProductAccessId", editAccessControlHandler)
}

func getCompanysProductsHandler(c *gin.Context) {
	thisCompany, err := lookupCompany(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	thisUser, _ := lookupThisUser(c)

	cps, err := getCompanysProducts(thisUser, thisCompany.ID)
	if err != nil {
		ErrorLog.Println("err getCompanysProducts: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	thisCoPublic := CompanyPublic{
		OPID:      thisCompany.OPID,
		Name:      thisCompany.Name,
		ShortName: thisCompany.ShortName,
	}

	resp := CompanyProductsResponse{Products: cps, ThisCompany: thisCoPublic, ThisUser: thisUser}

	c.JSON(200, resp)
	return
}

func updateCompanysProductListHandler(c *gin.Context) {
	user, err := lookupThisUser(c)
	if err != nil || !user.IsSystemAdmin {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	thisCompany, err := lookupCompany(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	input := &CompanyProductListUpdateRequest{}
	if err := c.ShouldBindWith(&input, binding.JSON); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	if input.ProductURL == "" || (input.ChangeType != "ADD" && input.ChangeType != "DELETE") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	alreadyExists := false
	existstingCP, err := getCompanyProductByURL(thisCompany.ID, input.ProductURL)
	if err == nil {
		alreadyExists = true
	}

	if input.ChangeType == "ADD" {
		if alreadyExists {
			enabled := true
			existstingCP.Enabled = &enabled
			dbmap.Update(&existstingCP)
		} else {
			product, err := lookupProductByURL(input.ProductURL)
			if err != nil {
				ErrorLog.Println("updateCompanysProductListHandler err lookupProductByURL:", err)
				c.JSON(http.StatusBadRequest, gin.H{"error": "Could not find that Product"})
				return
			}

			// need to set integrations manually because for jobs and userprov, we used to have to manually add integrations
			// but with products after, we went away from products/integrations model, just used products
			integrations := []string{}
			switch product.URL {
			case PRODUCT_USERPROVISIONING_URL:
				integrations = []string{gSuiteIntegrationsURL, m365IntegrationsURL, sfIntegrationsURL, boxIntegrationsURL, dropboxIntegrationsURL}
			case JOBS_PRODUCT_URL:
				integrations = []string{INDEED_INTEGRATION_URL, ZIPRECRUITER_INTEGRATION_URL}
			}

			_, err = addCompanyProduct(thisCompany.ID, product, nil, true, integrations)
			if err != nil {
				ErrorLog.Println("updateCompanysProductListHandler err addCompanyProduct:", err)
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		}
	} else {
		if alreadyExists {
			enabled := false
			existstingCP.Enabled = &enabled
			dbmap.Update(&existstingCP)
		}
	}

	cps, err := getCompanysProducts(user, thisCompany.ID)

	thisCoPublic := CompanyPublic{
		OPID:      thisCompany.OPID,
		Name:      thisCompany.Name,
		ShortName: thisCompany.ShortName,
	}

	resp := CompanyProductsResponse{Products: cps, ThisCompany: thisCoPublic, ThisUser: user}

	c.JSON(200, resp)
}

func getCompanysProducts(requestingUser *User, companyID int64) ([]CompanyProductJoin, error) {
	compProds := []CompanyProductJoin{}
	qry := `SELECT p.name name, p.description description, p.url url, cps.id company_product_id, cps.settings settings, cps.enabled enabled
          FROM products p, company_products cps
          WHERE p.id = cps.product_id AND cps.company_id = ? AND cps.enabled = 1`
	_, err := dbmap.Select(&compProds, qry, companyID)
	if err != nil {
		return compProds, err
	}

	for index := 0; index < len(compProds); index++ {
		cis, err := getCompanyProductIntegrations(compProds[index].CompanyProductID)
		if err != nil {
			ErrorLog.Println("err getting company ints: comp prod id: ", compProds[index].CompanyProductID)
			return compProds, err
		}

		compProds[index].Integrations = cis

		acs, err := getCompanyProductsAccessControls(false, compProds[index].CompanyProductID)
		if err != nil {
			ErrorLog.Println("err getCompanyProductsAccessControls: ", err)
			return compProds, err
		}

		if requestingUser != nil {
			for _, ac := range acs {
				if ac.Active && ac.EmployeeOPID == requestingUser.OPID {
					auth := &Authorization{
						Read:  ac.AccessType == "R" || ac.AccessType == "W",
						Write: ac.AccessType == "W",
					}
					compProds[index].Authorization = auth
				}
			}
		}

		if requestingUser != nil && (requestingUser.IsSystemAdmin || requestingUser.IsCompanyAdmin) {
			auth := &Authorization{
				Read:  true,
				Write: true,
			}
			compProds[index].Authorization = auth
			compProds[index].AccessControls = &acs
		}
	}

	return compProds, nil
}

func getCompanyProductByURL(companyID int64, productURL string) (comProd CompanyProduct, err error) {
	prod, err := lookupProductByURL(productURL)
	if err != nil {
		return
	}

	comProd, err = getCompanyProduct(companyID, prod.ID)

	return
}

func getCompanyProduct(companyID, productID int64) (thisCompanyProduct CompanyProduct, err error) {
	err = dbmap.SelectOne(&thisCompanyProduct, "SELECT * FROM company_products WHERE company_id = ? AND product_id = ?", companyID, productID)
	return
}

func getCompanyProductById(companyProductID int64) (thisCompanyProduct CompanyProduct, err error) {
	err = dbmap.SelectOne(&thisCompanyProduct, "SELECT * FROM company_products WHERE id = ?", companyProductID)
	return
}

func addCompanyProductHandler(c *gin.Context) {
	err := isAdminUser(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	input := &CompanyProductsPostRequest{}
	if err := c.ShouldBindWith(&input, binding.JSON); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	if input.CompanyID == 0 || input.ProductURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	product, err := lookupProductByURL(input.ProductURL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Could not find that Product"})
		return
	}

	_, err = addCompanyProduct(input.CompanyID, product, input.Settings, true, input.Integrations)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user := &User{}
	user.IsSystemAdmin = true

	cps, err := getCompanysProducts(user, input.CompanyID)

	c.JSON(200, cps)
}

func addCompanyProduct(companyID int64, product Product, settings *PropertyMap, enabled bool, integrations []string) (*CompanyProduct, error) {
	newComProd := &CompanyProduct{
		CompanyID: companyID,
		ProductID: product.ID,
		Enabled:   &enabled,
		Settings:  product.DefaultSettings,
	}

	if settings != nil {
		newComProd.Settings = *settings
	}

	err := dbmap.Insert(newComProd)
	if err != nil {
		ErrorLog.Println(err)
		return newComProd, err
	}

	for _, integrationURL := range integrations {
		_, err := createCompanyIntegrationIfNotExists(newComProd.ID, integrationURL, companyID, nil)
		if err != nil {
			ErrorLog.Println("could not add that Company Integration: ", integrationURL)
			continue
		}
	}

	return newComProd, nil
}

func updateCompanyProductHandler(c *gin.Context) {
	thisCompany, err := lookupCompany(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	input := PropertyMap{}
	if err := c.ShouldBindWith(&input, binding.JSON); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	companyProductId := c.Param("companyProductId")
	if companyProductId == "" {
		ErrorLog.Println("blank companyProductId path parameter")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	thisCompanyProduct := CompanyProduct{}
	err = dbmap.SelectOne(&thisCompanyProduct, "SELECT * FROM company_products WHERE id = ?", companyProductId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not found"})
		return
	}

	thisCompanyProduct.Settings = input

	_, err = dbmap.Update(&thisCompanyProduct)
	if err != nil {
		ErrorLog.Println("err updating: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Could not update"})
		return
	}

	// TODO: this is a temporary solution, eventually this should use access control too
	user := &User{}
	user.IsSystemAdmin = true

	// return all because of redux shortcut, need to eventually return only this one
	cps, err := getCompanysProducts(user, thisCompany.ID)
	if err != nil {
		ErrorLog.Println("err getCompanysProducts: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "An error occured"})
		return
	}

	c.JSON(http.StatusOK, cps)
}

func convertEmailSettingToArray() {
	allCompanyProducts := []CompanyProduct{}
	_, err := dbmap.Select(&allCompanyProducts, "SELECT * FROM company_products")
	if err != nil {
		ErrorLog.Println("err convertEmailSettingToArray: ", err)
		return
	}

	for _, cp := range allCompanyProducts {
		neSetting, ok := cp.Settings["notification_email_recipient"]
		if !ok {
			continue
		}

		newArr := []map[string]interface{}{}

		if neSetting != nil {
			neSettingInterface, ok := neSetting.(map[string]interface{})
			if !ok {
				ErrorLog.Printf("err asserting neSetting %+v\n", neSetting)
				continue
			}

			newArr = append(newArr, neSettingInterface)
		}

		cp.Settings["notification_email_recipient"] = newArr
		_, err = dbmap.Update(&cp)
		if err != nil {
			ErrorLog.Println("err convertEmailSettingToArray Update cpid: ", cp.ID, " err: ", err)
			continue
		}
	}
}

// could be extended to all products
func findCompaniesWithAtLeastOneActiveIntegrationInProduct(productURL string) ([]Company, error) {
	product, err := lookupProductByURL(productURL)
	if err != nil {
		return []Company{}, errors.New("cannot lookup jobs product")
	}

	qry := "SELECT cps.* FROM companies c, company_products cps WHERE cps.product_id = ? AND cps.company_id = c.id"
	allRecruitmentCompanyProducts := []CompanyProduct{}
	_, err = dbmap.Select(&allRecruitmentCompanyProducts, qry, product.ID)
	if err != nil {
		return []Company{}, errors.New("cannot lookup allRecruitmentCompanyProducts")
	}

	activeCompanies := []Company{}
	for _, cp := range allRecruitmentCompanyProducts {
		qry = "SELECT cis.* FROM company_integrations cis, company_product_assignments cpas WHERE cpas.company_product_id = ? AND cis.id = cpas.company_integration_id"
		allRecruitmentCompanyIntegrations := []CompanyIntegration{}
		_, err = dbmap.Select(&allRecruitmentCompanyIntegrations, qry, cp.ID)
		if err != nil {
			return activeCompanies, errors.New(fmt.Sprintf("cannot lookup allRecruitmentCompanyIntegrations: company id: %d", cp.CompanyID))
		}

		atLeastOneBoardActive := false
		for _, ci := range allRecruitmentCompanyIntegrations {
			enabled := checkIntegrationEnabled(ci)
			if enabled {
				atLeastOneBoardActive = true
			}
		}

		if atLeastOneBoardActive {
			company, err := lookupCompanyByID(cp.CompanyID)
			if err != nil {
				ErrorLog.Println("lookupCompanyByID err: " + err.Error())
				return activeCompanies, err
			}

			activeCompanies = append(activeCompanies, company)
		}
	}

	return activeCompanies, nil
}

func findAllCompaniesWithProduct(productURL string) ([]CompanyProduct, error) {
	allCompanyProducts := []CompanyProduct{}

	product, err := lookupProductByURL(productURL)
	if err != nil {
		return allCompanyProducts, errors.New("cannot lookup product")
	}

	qry := "SELECT * FROM company_products cps WHERE cps.product_id = ? AND enabled = 1"
	_, err = dbmap.Select(&allCompanyProducts, qry, product.ID)
	if err != nil {
		return allCompanyProducts, errors.New("cannot lookup allRecruitmentCompanyProducts")
	}

	return allCompanyProducts, nil
}
