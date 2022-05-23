package main

import (
	"os"
	"strconv"
)

func runScripts() {
	runCrons := os.Getenv("CRONS")
	if runCrons == "on" {
		go startCrons()
	}

	simulate := os.Getenv("SIMULATE")
	if simulate != "" {
		accountID := os.Getenv("ACCOUNTID")
		intID, _ := strconv.ParseInt(accountID, 10, 64)

		if simulate == "hire" {
			simulateHiring(true, intID)
		} else if simulate == "fire" {
			simulateHiring(false, intID)
		}
	}

	prodSettingURL := os.Getenv("PRODUCTSETTINGURL")
	if prodSettingURL != "" {
		prodSettingName := os.Getenv("PRODUCTSETTINGNAME")
		if prodSettingName != "" {
			prodSettingValue := os.Getenv("PRODUCTSETTINGVALUE")
			if prodSettingValue != "" {
				valuePtr := &prodSettingValue
				if prodSettingValue == "null" {
					valuePtr = nil
				}

				err := addProductSettingIfNotExists(prodSettingURL, prodSettingName, valuePtr)
				if err != nil {
					ErrorLog.Println("product setting script ERR! ", err)
				}
			}
		}
	}

	integrationSettingURL := os.Getenv("INTSETTINGURL")
	if integrationSettingURL != "" {
		integrationSettingName := os.Getenv("INTSETTINGNAME")
		if integrationSettingName != "" {
			integrationSettingValue := os.Getenv("INTSETTINGVALUE")
			if integrationSettingValue != "" {
				valuePtr := &integrationSettingValue
				if integrationSettingValue == "null" {
					valuePtr = nil
				}

				err := addIntegrationSettingIfNotExists(integrationSettingURL, integrationSettingName, valuePtr)
				if err != nil {
					ErrorLog.Println("integration setting script ERR! ", err)
				}
			}
		}
	}

	testEmail := os.Getenv("TESTEMAIL")
	if testEmail != "" {
		sendTestEmail()
	}

	convertEnabled := os.Getenv("CONVERTENABLED")
	if convertEnabled != "" {
		convertCIEnabledToSetting()
	}

	convertEmail := os.Getenv("CONVERTEMAIL")
	if convertEmail != "" {
		convertEmailSettingToArray()
	}
}
