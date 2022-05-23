package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

type CompanyIntegration struct {
	ID             int64       `db:"id, primarykey, autoincrement"`
	CompanyID      int64       `db:"company_id" json:"company_id"`
	IntegrationID  int64       `db:"integration_id" json:"integration_id"`
	Status         string      `db:"status" json:"status"`
	AuthedUsername string      `db:"authed_username" json:"authed_username"`
	Enabled        *bool       `db:"enabled" json:"enabled"`
	Created        int64       `db:"created" json:"-"`
	Settings       PropertyMap `db:"settings,size:1024" json:"settings"`
}

type CompanyIntegrationPublic struct {
	IntegrationName string      `json:"integration_name"`
	Authenticated   bool        `json:"authenticated"`
	Description     string      `json:"description"`
	URL             string      `json:"url"`
	AuthURL         string      `json:"auth_url"`
	ImageName       string      `json:"image_name"`
	AuthedUsername  string      `json:"authed_username"`
	Enabled         *bool       `json:"enabled"`
	ID              int64       `json:"-"`
	IntegrationID   int64       `json:"-"`
	Settings        PropertyMap `json:"settings"`
}

type EmailProvidersInFront []CompanyIntegrationPublic

func (a EmailProvidersInFront) Len() int      { return len(a) }
func (a EmailProvidersInFront) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a EmailProvidersInFront) Less(i, j int) bool {
	if a[i].URL == gSuiteIntegrationsURL || a[i].URL == m365IntegrationsURL {
		return true
	} else {
		return false
	}
}

type MyIntegrationsResponse struct {
	Integrations []CompanyIntegrationPublic `json:"integrations"`
	ThisCompany  CompanyPublic              `json:"company"`
}

type UpdateMyIntegrationSettingsRequest struct {
	IntegrationURL string `json:"url"`
	NewEnabled     string `json:"new_enabled"`
}

type DisconnectIntegrationRequest struct {
	IntegrationURL string `json:"url"`
}

type CompanyIntegrationPostRequest struct {
	CompanyID      int64        `json:"company_id"`
	ProductURL     string       `json:"product_url"`
	IntegrationURL string       `json:"integration_url"`
	Settings       *PropertyMap `json:"settings"`
}

func registerCompanyIntegrationRoutes(router *gin.Engine) {
	router.POST("/api/companyintegration", addCompanyIntegrationHandler)
	router.POST("/api/my/integrations/disconnect", disconnectIntegration)
	router.POST("/api/companyintegration/:companyIntegrationURL/settings/update", updateSettingsHandler)
}

func updateSettingsHandler(c *gin.Context) {
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

	companyIntegrationURL := c.Param("companyIntegrationURL")
	if companyIntegrationURL == "" {
		ErrorLog.Println("blank companyIntegrationURL path parameter")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	thisCompanyIntegration, err := getCompanyIntegrationByIntegrationString(companyIntegrationURL, thisCompany.ID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	thisCompanyIntegration.Settings = input

	_, err = dbmap.Update(&thisCompanyIntegration)
	if err != nil {
		ErrorLog.Println("err updating: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Could not update"})
		return
	}

	publicCI := transformCIToPublic(thisCompanyIntegration)

	c.JSON(200, publicCI)
}

func disconnectIntegration(c *gin.Context) {
	thisCompany, err := lookupCompany(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	input := &DisconnectIntegrationRequest{}
	if err := c.ShouldBindWith(&input, binding.JSON); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	if input.IntegrationURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	existingCompanyInt, err := getCompanyIntegrationByIntegrationString(input.IntegrationURL, thisCompany.ID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot find that integration"})
		return
	}

	err = deleteOAuthByCompanyIntegrationID(existingCompanyInt.ID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot disconnect that integration"})
		return
	}

	existingCompanyInt.Status = "disconnected"
	existingCompanyInt.AuthedUsername = ""

	_, err = dbmap.Update(&existingCompanyInt)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot update"})
		return
	}

	publicCI := transformCIToPublic(existingCompanyInt)

	c.JSON(200, publicCI)
}

func updateEnabledHandler(c *gin.Context) {
	thisCompany, err := lookupCompany(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	input := &UpdateMyIntegrationSettingsRequest{}
	if err := c.ShouldBindWith(&input, binding.JSON); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	if input.NewEnabled == "" || input.IntegrationURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	existingCompanyInt, err := getCompanyIntegrationByIntegrationString(input.IntegrationURL, thisCompany.ID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot find that integration"})
		return
	}

	if input.NewEnabled == "enabled" {
		*existingCompanyInt.Enabled = true
	} else if input.NewEnabled == "disabled" {
		*existingCompanyInt.Enabled = false
	}

	_, err = dbmap.Update(&existingCompanyInt)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot update"})
		return
	}

	publicCI := transformCIToPublic(existingCompanyInt)

	c.JSON(200, publicCI)
}

func addCompanyIntegrationHandler(c *gin.Context) {
	err := isAdminUser(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	input := CompanyIntegrationPostRequest{}
	if err := c.ShouldBindWith(&input, binding.JSON); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	if input.CompanyID == 0 || input.IntegrationURL == "" || input.ProductURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	comProd, err := getCompanyProductByURL(input.CompanyID, input.ProductURL)
	if err != nil {
		ErrorLog.Println("err getCompanyProduct: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Could not lookup that company product"})
		return
	}

	_, err = createCompanyIntegrationIfNotExists(comProd.ID, input.IntegrationURL, input.CompanyID, input.Settings)
	if err != nil {
		ErrorLog.Println("could not add that Company Integration: ", input.IntegrationURL, " for co: ", input.CompanyID)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, input)
}

func getCompanyProductIntegrations(companyProductID int64) ([]CompanyIntegrationPublic, error) {
	allCompanyIntegrations := []CompanyIntegration{}
	allPublicCompanyIntegrations := []CompanyIntegrationPublic{}

	qry := `SELECT cis.* FROM company_integrations cis, company_product_assignments cpas
					WHERE cpas.company_integration_id = cis.id AND cpas.company_product_id = ?`
	_, err := dbmap.Select(&allCompanyIntegrations, qry, companyProductID)
	if err != nil {
		ErrorLog.Println("err looking up all integrations: ", err)
		return allPublicCompanyIntegrations, err
	}

	for _, ci := range allCompanyIntegrations {
		publicCI := transformCIToPublic(ci)

		allPublicCompanyIntegrations = append(allPublicCompanyIntegrations, publicCI)
	}

	return allPublicCompanyIntegrations, nil
}

func createCompanyIntegrationIfNotExists(companyProductID int64, integrationURL string, companyID int64, settings *PropertyMap) (*CompanyIntegration, error) {
	intx, err := lookupIntegrationByURL(integrationURL)
	if err != nil {
		ErrorLog.Println("could not lookup that Integration: ", err)
		return &CompanyIntegration{}, errors.New("Could not find that Integration by URL")
	}

	ciExiting, err := getCompanyIntegration(intx.ID, companyID)
	if err == nil {
		return &ciExiting, nil
	}

	newCI := &CompanyIntegration{IntegrationID: intx.ID, CompanyID: companyID}
	newCI.Created = time.Now().Unix()
	newCI.Status = "new"
	newCI.Settings = intx.DefaultSettings

	if settings != nil {
		newCI.Settings = *settings
	}

	err = dbmap.Insert(newCI)
	if err != nil {
		ErrorLog.Println(err)
		return newCI, err
	}

	newCompProdAssignment := &CompanyProductAssignment{
		CompanyProductID:     companyProductID,
		CompanyIntegrationID: newCI.ID,
	}
	err = dbmap.Insert(newCompProdAssignment)
	if err != nil {
		return newCI, errors.New("error: Could not insert newCompProdAssignment: " + err.Error())
	}

	return newCI, err
}

func transformCIToPublic(ci CompanyIntegration) CompanyIntegrationPublic {
	thisIntegration, _ := getIntegrationByID(ci.IntegrationID)

	publicCI := CompanyIntegrationPublic{
		IntegrationName: thisIntegration.Name,
		Description:     thisIntegration.Description,
		URL:             thisIntegration.URL,
		AuthedUsername:  ci.AuthedUsername,
		Enabled:         ci.Enabled,
		AuthURL:         thisIntegration.RedirectURL,
		ImageName:       thisIntegration.ImgURL,
		ID:              ci.ID,
		IntegrationID:   thisIntegration.ID,
		Settings:        ci.Settings,
	}

	if ci.Status == "connected" {
		publicCI.Authenticated = true
	} else {
		publicCI.Authenticated = false
	}

	return publicCI
}

func getCompanyIntegrationByIntegrationString(integration string, companyID int64) (CompanyIntegration, error) {
	intID, err := dbmap.SelectInt("SELECT id FROM integrations WHERE url=?", integration)
	if err != nil || intID == 0 {
		ErrorLog.Println("couldnt look up integration: ", err)
		return CompanyIntegration{}, errors.New("couldnt find integration")
	}

	ci, err := getCompanyIntegration(intID, companyID)
	return ci, err
}

func getCompanyIntegration(integrationID, companyID int64) (CompanyIntegration, error) {
	thisCompanyIntegration := CompanyIntegration{}
	err := dbmap.SelectOne(&thisCompanyIntegration, "SELECT * FROM company_integrations WHERE company_id = ? AND integration_id = ?", companyID, integrationID)
	if err != nil {
		return CompanyIntegration{}, err
	}

	return thisCompanyIntegration, nil
}

func getCompaniesCIByProduct(productID, companyID int64) ([]CompanyIntegrationPublic, error) {
	allCIs := []CompanyIntegration{}
	allPCIs := []CompanyIntegrationPublic{}

	qry := `SELECT cis.*
					FROM company_integrations cis, company_product_assignments cpas, company_products cps
					WHERE cpas.company_integration_id = cis.id AND cps.product_id = ? AND cps.id = cpas.company_product_id AND cps.company_id = ?`
	_, err := dbmap.Select(&allCIs, qry, productID, companyID)
	if err != nil {
		ErrorLog.Println("err looking up comapanies cis: ", err)
		ErrorLog.Println("comp id was: ", companyID)
		return allPCIs, err
	}

	for _, ci := range allCIs {
		pci := transformCIToPublic(ci)
		allPCIs = append(allPCIs, pci)
	}

	return allPCIs, nil
}

func convertCIEnabledToSetting() {
	allCIs := []CompanyIntegration{}
	_, err := dbmap.Select(&allCIs, "SELECT * FROM company_integrations")
	if err != nil {
		ErrorLog.Println("all ci lookup failed: ", err)
		return
	}

	for _, ci := range allCIs {
		ci.Settings["enabled"] = ci.Enabled
		_, err = dbmap.Update(&ci)
		if err != nil {
			ErrorLog.Println("update ci id:", ci.ID, " failed: ", err)
			continue
		}
	}
}

func addIntegrationSettingIfNotExists(intURL, settingName string, defaultValue interface{}) error {
	integration, err := lookupIntegrationByURL(intURL)
	if err != nil {
		ErrorLog.Println("err addIntegrationSettingIfNotExists cannot lookup int: ", err)
		return err
	}

	allCompanyIntegrations := []CompanyIntegration{}
	_, err = dbmap.Select(&allCompanyIntegrations, "SELECT cps.* FROM company_integrations cps, integrations i WHERE i.url = ? AND i.id = cps.integration_id", intURL)
	if err != nil {
		ErrorLog.Println("err addIntegrationSettingIfNotExists: ", err)
		return err
	}

	valueToUse := defaultValue

	defaultValueAsString, ok := defaultValue.(*string)
	if ok {
		if defaultValueAsString != nil {
			valueAsMap := make(map[string]interface{})
			err = json.Unmarshal([]byte(*defaultValueAsString), &valueAsMap)
			if err != nil {
				ErrorLog.Println("addIntegrationSettingIfNotExists failed to encode as STRUCT, using string")
			} else {
				valueToUse = valueAsMap
				InfoLog.Printf("using an object ! %+v", valueAsMap)
			}
		}
	}

	integration.DefaultSettings[settingName] = valueToUse
	_, err = dbmap.Update(&integration)
	if err != nil {
		ErrorLog.Println("err addIntegrationSettingIfNotExists Update int err: ", err)
		return err
	}

	for _, cp := range allCompanyIntegrations {
		if _, ok := cp.Settings[settingName]; ok {
			continue
		}

		cp.Settings[settingName] = valueToUse

		_, err = dbmap.Update(&cp)
		if err != nil {
			ErrorLog.Println("err addIntegrationSettingIfNotExists Update cpid: ", cp.ID, " err: ", err)
			return err
		}
	}

	return nil
}

func getCompanyIntegrationsUsingIntegrationURL(integrationURL string) (companyIntegrations []CompanyIntegration, err error) {
	thisIntegration, err := lookupIntegrationByURL(integrationURL)
	if err != nil {
		return
	}

	companyIntegrations, err = getCompanyIntegrationsUsingIntegrationID(thisIntegration.ID)

	return
}

func getCompanyIntegrationsUsingIntegrationID(integrationID int64) (companyIntegrations []CompanyIntegration, err error) {
	_, err = dbmap.Select(&companyIntegrations, "SELECT * FROM company_integrations cis WHERE cis.integration_id = ?", integrationID)
	return
}
