package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

type Integration struct {
	ID              int64       `db:"id, primarykey, autoincrement"`
	Name            string      `db:"name" json:"name"`
	Description     string      `db:"description" json:"description"`
	URL             string      `db:"url" json:"url"`
	ImgURL          string      `db:"img_url" json:"img_url"`
	RedirectURL     string      `db:"redirect_url" json:"redirect_url"`
	AccessURL       string      `db:"access_url" json:"access_url"`
	DefaultSettings PropertyMap `db:"default_settings" json:"default_settings"`
}

type AddIntegrationSettingBody struct {
	Name         string      `json:"name"`
	DefaultValue PropertyMap `json:"value"`
}

func registerIntegrationRoutes(router *gin.Engine) {
	router.POST("/api/integrations", addIntegration)
	router.POST("/api/integrations/:integrationURL/addSetting", addIntegrationSetting)
}

func addIntegration(c *gin.Context) {
	err := isAdminUser(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	input := Integration{}
	if err := c.ShouldBindWith(&input, binding.JSON); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	err = dbmap.Insert(&input)
	if err != nil {
		ErrorLog.Println(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Something went wrong"})
		return
	}

	c.JSON(http.StatusCreated, input)
}

func addIntegrationSetting(c *gin.Context) {
	err := isAdminUser(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	integrationURL := c.Param("integrationURL")
	if integrationURL == "" {
		ErrorLog.Println("blank integrationURL path parameter")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	input := AddIntegrationSettingBody{}
	if err := c.ShouldBindWith(&input, binding.JSON); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	err = addIntegrationSettingIfNotExists(integrationURL, input.Name, input.DefaultValue)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
}

func lookupIntegrationByURL(url string) (Integration, error) {
	thisIntegration := Integration{}
	err := dbmap.SelectOne(&thisIntegration, "SELECT * FROM integrations WHERE url = ?", url)
	if err != nil {
		ErrorLog.Println("couldnt look up integration: ", err)
		return thisIntegration, err
	}

	return thisIntegration, nil
}

func getIntegrationByID(id int64) (Integration, error) {
	thisIntegration := Integration{}
	err := dbmap.SelectOne(&thisIntegration, "SELECT * FROM integrations WHERE id = ?", id)
	if err != nil {
		ErrorLog.Println("couldnt look up integration: ", err)
		return Integration{}, err
	}

	return thisIntegration, nil
}
