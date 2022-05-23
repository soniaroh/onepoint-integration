package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

type Product struct {
	ID              int64       `db:"id, primarykey, autoincrement" json:"id"`
	Name            string      `db:"name" json:"name"`
	URL             string      `db:"url" json:"url"`
	Description     string      `db:"description" json:"description"`
	DefaultSettings PropertyMap `db:"default_settings" json:"default_settings"`
}

type AddProductSettingBody struct {
	Name         string      `json:"name"`
	DefaultValue PropertyMap `json:"value"`
}

const (
	PRODUCT_USERPROVISIONING_URL = "user-provisioning"
)

func registerProductRoutes(router *gin.Engine) {
	router.GET("/api/products", getCompanysProductsHandler)
	router.POST("/api/products", addProductHandler)
	router.POST("/api/products/:productURL/addSetting", addProductSettingHandler)
}

func lookupProductByURL(url string) (Product, error) {
	thisProd := Product{}
	err := dbmap.SelectOne(&thisProd, "SELECT * FROM products WHERE url = ?", url)
	return thisProd, err
}

func addProductHandler(c *gin.Context) {
	err := isAdminUser(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	input := &Product{}
	if err := c.ShouldBindWith(&input, binding.JSON); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	if input.Name == "" || input.Description == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	err = dbmap.Insert(input)
	if err != nil {
		ErrorLog.Println(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Something went wrong"})
		return
	}

	c.JSON(http.StatusCreated, input)
}

func addProductSettingHandler(c *gin.Context) {
	err := isAdminUser(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	productURL := c.Param("productURL")
	if productURL == "" {
		ErrorLog.Println("blank productURL path parameter")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	input := AddProductSettingBody{}
	if err := c.ShouldBindWith(&input, binding.JSON); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	err = addProductSettingIfNotExists(productURL, input.Name, input.DefaultValue)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
}

func addProductSettingIfNotExists(productURL, settingName string, defaultValue interface{}) error {
	product, err := lookupProductByURL(productURL)
	if err != nil {
		ErrorLog.Println("err addProductSettingIfNotExists cannot lookup product: ", err)
		return err
	}

	product.DefaultSettings[settingName] = defaultValue
	_, err = dbmap.Update(&product)
	if err != nil {
		ErrorLog.Println("err addProductSettingIfNotExists Update product err: ", err)
		return err
	}

	allCompanyProducts := []CompanyProduct{}
	_, err = dbmap.Select(&allCompanyProducts, "SELECT cps.* FROM company_products cps, products p WHERE p.url = ? AND p.id = cps.product_id", productURL)
	if err != nil {
		ErrorLog.Println("err addProductSettingIfNotExist: ", err)
		return err
	}

	for _, cp := range allCompanyProducts {
		if _, ok := cp.Settings[settingName]; ok {
			continue
		}

		cp.Settings[settingName] = defaultValue

		_, err = dbmap.Update(&cp)
		if err != nil {
			ErrorLog.Println("err addProductSettingIfNotExist Update cpid: ", cp.ID, " err: ", err)
			return err
		}
	}

	return nil
}
