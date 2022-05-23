package main

import "opapi"

// used for all clients
var adminCxn *opapi.OPConnection

// used for hired/fired reports in order to view OP internal, but limited emp info
var hfReportsCxn *opapi.OPConnection

// used for OP internal changes/employee details
var opInternalCxn *opapi.OPConnection

const (
	OP_INTERNAL_COMPANY_ID = 82890
)

func chooseOPAPICxn(opCompanyID int64) *opapi.OPConnection {
	cxnToUse := adminCxn

	if opCompanyID == OP_INTERNAL_COMPANY_ID {
		cxnToUse = opInternalCxn
	}

	return cxnToUse
}

func initOPAPIConnections() {
	adminCreds := opapi.OPCredentials{
		Username:         passwords.OP_ADMIN_USERNAME,
		Password:         passwords.OP_ADMIN_PASSWORD,
		CompanyShortname: passwords.OP_ADMIN_SHORTNAME,
		APIKey:           passwords.OP_ADMIN_APIKEY,
	}

	adminCxn = &opapi.OPConnection{
		Credentials: &adminCreds,
	}

	hfReportsCreds := opapi.OPCredentials{
		Username:         passwords.OP_INTERNAL_ADMIN_USERNAME,
		Password:         passwords.OP_INTERNAL_ADMIN_PASSWORD,
		CompanyShortname: passwords.OP_ADMIN_SHORTNAME,
		APIKey:           passwords.OP_ADMIN_APIKEY,
	}

	hfReportsCxn = &opapi.OPConnection{
		Credentials: &hfReportsCreds,
	}

	opInternalCreds := opapi.OPCredentials{
		Username:         passwords.OP_INTERNAL_USERNAME,
		Password:         passwords.OP_INTERNAL_PASSWORD,
		CompanyShortname: passwords.OP_INTERNAL_SHORTNAME,
		APIKey:           passwords.OP_INTERNAL_APIKEY,
	}

	opInternalCxn = &opapi.OPConnection{
		Credentials: &opInternalCreds,
	}
}
