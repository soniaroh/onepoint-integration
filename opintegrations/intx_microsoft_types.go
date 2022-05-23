package main

type M365PasswordProfile struct {
	ForceChangePasswordNextSignIn bool   `json:"forceChangePasswordNextSignIn"`
	Password                      string `json:"password"`
}

type M365ErrorResponseBody struct {
	Error M365ErrorResponse `json:"error"`
}

type M365ErrorResponse struct {
	Code       string                   `json:"code"`
	Message    string                   `json:"message"`
	InnerError M365InnerError           `json:"innerError"`
	Details    []map[string]interface{} `json:"details"`
}

type M365InnerError struct {
	RequestID string `json:"request-id"`
	Date      string `json:"date"`
}

type M365NewUserRequest struct {
	AccountEnabled    bool                `json:"accountEnabled,omitempty"`
	City              string              `json:"city,omitempty"`
	Country           string              `json:"country,omitempty"`
	DisplayName       string              `json:"displayName,omitempty"`
	GivenName         string              `json:"givenName,omitempty"`
	MailNickname      string              `json:"mailNickname,omitempty"`
	MobilePhone       string              `json:"mobilePhone,omitempty"`
	PasswordProfile   M365PasswordProfile `json:"passwordProfile,omitempty"`
	PostalCode        string              `json:"postalCode,omitempty"`
	Surname           string              `json:"surname,omitempty"`
	State             string              `json:"state,omitempty"`
	StreetAddress     string              `json:"streetAddress,omitempty"`
	UserPrincipalName string              `json:"userPrincipalName,omitempty"`
}

type M365UserResource struct {
	AboutMe          string `json:"aboutMe"`
	AccountEnabled   bool   `json:"accountEnabled"`
	AssignedLicenses []struct {
		OdataType string `json:"@odata.type"`
	} `json:"assignedLicenses"`
	AssignedPlans []struct {
		OdataType string `json:"@odata.type"`
	} `json:"assignedPlans"`
	Birthday        string   `json:"birthday"`
	BusinessPhones  []string `json:"businessPhones"`
	City            string   `json:"city"`
	CompanyName     string   `json:"companyName"`
	Country         string   `json:"country"`
	Department      string   `json:"department"`
	DisplayName     string   `json:"displayName"`
	GivenName       string   `json:"givenName"`
	HireDate        string   `json:"hireDate"`
	ID              string   `json:"id"`
	Interests       []string `json:"interests"`
	JobTitle        string   `json:"jobTitle"`
	Mail            string   `json:"mail"`
	MailboxSettings struct {
		OdataType string `json:"@odata.type"`
	} `json:"mailboxSettings"`
	MailNickname                 string              `json:"mailNickname"`
	MobilePhone                  string              `json:"mobilePhone"`
	MySite                       string              `json:"mySite"`
	OfficeLocation               string              `json:"officeLocation"`
	OnPremisesImmutableID        string              `json:"onPremisesImmutableId"`
	OnPremisesLastSyncDateTime   string              `json:"onPremisesLastSyncDateTime"`
	OnPremisesSecurityIdentifier string              `json:"onPremisesSecurityIdentifier"`
	OnPremisesSyncEnabled        bool                `json:"onPremisesSyncEnabled"`
	PasswordPolicies             string              `json:"passwordPolicies"`
	PasswordProfile              M365PasswordProfile `json:"passwordProfile"`
	PastProjects                 []string            `json:"pastProjects"`
	PostalCode                   string              `json:"postalCode"`
	PreferredLanguage            string              `json:"preferredLanguage"`
	PreferredName                string              `json:"preferredName"`
	ProvisionedPlans             []struct {
		OdataType string `json:"@odata.type"`
	} `json:"provisionedPlans"`
	ProxyAddresses    []string `json:"proxyAddresses"`
	Responsibilities  []string `json:"responsibilities"`
	Schools           []string `json:"schools"`
	Skills            []string `json:"skills"`
	State             string   `json:"state"`
	StreetAddress     string   `json:"streetAddress"`
	Surname           string   `json:"surname"`
	UsageLocation     string   `json:"usageLocation"`
	UserPrincipalName string   `json:"userPrincipalName"`
	UserType          string   `json:"userType"`
	Calendar          struct {
		OdataType string `json:"@odata.type"`
	} `json:"calendar"`
	CalendarGroups []struct {
		OdataType string `json:"@odata.type"`
	} `json:"calendarGroups"`
	CalendarView []struct {
		OdataType string `json:"@odata.type"`
	} `json:"calendarView"`
	Calendars []struct {
		OdataType string `json:"@odata.type"`
	} `json:"calendars"`
	Contacts []struct {
		OdataType string `json:"@odata.type"`
	} `json:"contacts"`
	ContactFolders []struct {
		OdataType string `json:"@odata.type"`
	} `json:"contactFolders"`
	CreatedObjects []struct {
		OdataType string `json:"@odata.type"`
	} `json:"createdObjects"`
	DirectReports []struct {
		OdataType string `json:"@odata.type"`
	} `json:"directReports"`
	Drive struct {
		OdataType string `json:"@odata.type"`
	} `json:"drive"`
	Drives []struct {
		OdataType string `json:"@odata.type"`
	} `json:"drives"`
	Events []struct {
		OdataType string `json:"@odata.type"`
	} `json:"events"`
	InferenceClassification struct {
		OdataType string `json:"@odata.type"`
	} `json:"inferenceClassification"`
	MailFolders []struct {
		OdataType string `json:"@odata.type"`
	} `json:"mailFolders"`
	Manager struct {
		OdataType string `json:"@odata.type"`
	} `json:"manager"`
	MemberOf []struct {
		OdataType string `json:"@odata.type"`
	} `json:"memberOf"`
	Messages []struct {
		OdataType string `json:"@odata.type"`
	} `json:"messages"`
	OwnedDevices []struct {
		OdataType string `json:"@odata.type"`
	} `json:"ownedDevices"`
	OwnedObjects []struct {
		OdataType string `json:"@odata.type"`
	} `json:"ownedObjects"`
	Photo struct {
		OdataType string `json:"@odata.type"`
	} `json:"photo"`
	RegisteredDevices []struct {
		OdataType string `json:"@odata.type"`
	} `json:"registeredDevices"`
}

type M365SkusResponse struct {
	OdataContext string `json:"@odata.context"`
	Value        []struct {
		CapabilityStatus string `json:"capabilityStatus"`
		ConsumedUnits    int    `json:"consumedUnits"`
		ID               string `json:"id"`
		PrepaidUnits     struct {
			Enabled   int `json:"enabled"`
			Suspended int `json:"suspended"`
			Warning   int `json:"warning"`
		} `json:"prepaidUnits"`
		ServicePlans []struct {
			ServicePlanID      string `json:"servicePlanId"`
			ServicePlanName    string `json:"servicePlanName"`
			ProvisioningStatus string `json:"provisioningStatus"`
			AppliesTo          string `json:"appliesTo"`
		} `json:"servicePlans"`
		SkuID         string `json:"skuId"`
		SkuPartNumber string `json:"skuPartNumber"`
		AppliesTo     string `json:"appliesTo"`
	} `json:"value"`
}
