package main

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
)

type Passwords struct {
	ADMIN_KEY                        string `json:"admin_key"`
	PROD_DB_PW                       string `json:"prod_db_pw"`
	LOCAL_DB_PW                      string `json:"local_db_pw"`
	EMAIL_TEMP_PASSWORD              string `json:"email_temp_password"`
	GSUITE_API_SECRET                string `json:"gsuite_secret"`
	M365_API_SECRET                  string `json:"m365_secret"`
	SF_API_SECRET                    string `json:"sf_secret"`
	BOX_API_SECRET                   string `json:"box_secret"`
	DROPBOX_API_SECRET               string `json:"dropbox_secret"`
	OP_ADMIN_APIKEY                  string `json:"op_admin_apikey"`
	OP_ADMIN_SHORTNAME               string `json:"op_admin_shortname"`
	OP_ADMIN_USERNAME                string `json:"op_admin_username"`
	OP_ADMIN_PASSWORD                string `json:"op_admin_password"`
	OP_INTERNAL_ADMIN_USERNAME       string `json:"op_internal_admin_username"`
	OP_INTERNAL_ADMIN_PASSWORD       string `json:"op_internal_admin_password"`
	OP_INTERNAL_APIKEY               string `json:"op_internal_apikey"`
	OP_INTERNAL_SHORTNAME            string `json:"op_internal_shortname"`
	OP_INTERNAL_USERNAME             string `json:"op_internal_username"`
	OP_INTERNAL_PASSWORD             string `json:"op_internal_password"`
	NO_REPLY_EMAILER_ADDRESS         string `json:"no_reply_emailer_address"`
	ADMIN_NOTIFICATION_EMAIL_ADDRESS string `json:"admin_notification_email_address"`
	SG_EMAILER_PASSWORD              string `json:"sg_emailer_password"`
	ASSURE_HIRE_TOKEN                string `json:"assure_hire_token"`
}

var passwords Passwords

func loadPasswords() {
	absPath := "/etc/opintegrations/config/passwords.json"
	if !env.Production {
		absPath, _ = filepath.Abs("./opintegrations/config/passwords.json")
	}

	raw, err := ioutil.ReadFile(absPath)
	if err != nil {
		ErrorLog.Println(err)
		panic("FAILED to open password json: " + err.Error())
	}

	err = json.Unmarshal(raw, &passwords)
	if err != nil {
		ErrorLog.Println(err)
		panic("FAILED Unmarshal password json: " + err.Error())
	}
}
