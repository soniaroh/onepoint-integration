package main

type GSuiteErrorResponseBody struct {
	Error GSuiteErrorResponse `json:"error"`
}

type GSuiteErrorResponse struct {
	Code    float64                  `json:"code"`
	Message string                   `json:"message"`
	Errors  []map[string]interface{} `json:"errors"`
}

type GSuiteCreateUserBody struct {
	Name                    GSuiteNameResource `json:"name"`
	Password                string             `json:"password"`
	PrimaryEmail            string             `json:"primaryEmail"`
	Addresses               []GSuiteAddress    `json:"addresses,omitempty"`
	Phones                  []GSuitePhone      `json:"phones,omitempty"`
	ChangePasswordNextLogin bool               `json:"changePasswordAtNextLogin,omitempty"`
}

type GSuiteAddress struct {
	State         string `json:"region"`
	Locality      string `json:"locality"`
	PostalCode    string `json:"postalCode"`
	StreetAddress string `json:"streetAddress"`
	Type          string `json:"type"`
	CustomType    string `json:"customType"`
}

type GSuitePhone struct {
	Primary bool   `json:"primary,omitempty"`
	Type    string `json:"type,omitempty"`
	Value   string `json:"value,omitempty"`
}

type GSuiteUndeleteUserBody struct {
	OrgUnitPath string `json:"orgUnitPath"`
}

type GSuiteGetUsersResponse struct {
	Kind          string               `json:"kind"`
	Etag          string               `json:"etag"`
	Users         []GSuiteUserResource `json:"users"`
	NextPageToken string               `json:"nextPageToken"`
}

type GSuiteEmailResource struct {
	Address    string `json:"address"`
	Type       string `json:"type"`
	CustomType string `json:"customType"`
	Primary    bool   `json:"primary"`
}

type GSuiteGenderResource struct {
	Type         string `json:"type"`
	CustomGender string `json:"customGender"`
	AddressMeAs  string `json:"addressMeAs"`
}

type GSuiteNameResource struct {
	GivenName  string `json:"givenName"`
	FamilyName string `json:"familyName"`
	FullName   string `json:"fullName"`
}

type GSuiteGetUserEmailResponse struct {
	ResourceName   string `json:"resourceName"`
	Etag           string `json:"etag"`
	EmailAddresses []struct {
		Metadata struct {
			Primary  bool `json:"primary"`
			Verified bool `json:"verified"`
			Source   struct {
				Type string `json:"type"`
				ID   string `json:"id"`
			} `json:"source"`
		} `json:"metadata"`
		Value string `json:"value"`
	} `json:"emailAddresses"`
}

type GSuiteGetUserEmailResponseV2 struct {
	Email string `json:"email"`
}

type GSuiteUserResource struct {
	Kind                      string                `json:"kind"`
	ID                        string                `json:"id"`
	Etag                      string                `json:"etag"`
	PrimaryEmail              string                `json:"primaryEmail"`
	Name                      GSuiteNameResource    `json:"name"`
	IsAdmin                   bool                  `json:"isAdmin"`
	IsDelegatedAdmin          bool                  `json:"isDelegatedAdmin"`
	LastLoginTime             string                `json:"lastLoginTime"`
	CreationTime              string                `json:"creationTime"`
	DeletionTime              string                `json:"deletionTime"`
	AgreedToTerms             bool                  `json:"agreedToTerms"`
	Password                  string                `json:"password"`
	HashFunction              string                `json:"hashFunction"`
	Suspended                 bool                  `json:"suspended"`
	SuspensionReason          string                `json:"suspensionReason"`
	ChangePasswordAtNextLogin bool                  `json:"changePasswordAtNextLogin"`
	IPWhitelisted             bool                  `json:"ipWhitelisted"`
	Ims                       []struct{}            `json:"ims"`
	Emails                    []GSuiteEmailResource `json:"emails"`
	ExternalIds               []struct {
		Value      string `json:"value"`
		Type       string `json:"type"`
		CustomType string `json:"customType"`
	} `json:"externalIds"`
	Relations                  []struct{}           `json:"relations"`
	Addresses                  []GSuiteAddress      `json:"addresses"`
	Organizations              []struct{}           `json:"organizations"`
	Phones                     []GSuitePhone        `json:"phones"`
	Languages                  []struct{}           `json:"languages"`
	PosixAccounts              []struct{}           `json:"posixAccounts"`
	SSHPublicKeys              []struct{}           `json:"sshPublicKeys"`
	Aliases                    []string             `json:"aliases"`
	NonEditableAliases         []string             `json:"nonEditableAliases"`
	Notes                      struct{}             `json:"notes"`
	Websites                   []struct{}           `json:"websites"`
	Locations                  []struct{}           `json:"locations"`
	Keywords                   []struct{}           `json:"keywords"`
	Gender                     GSuiteGenderResource `json:"gender"`
	CustomerID                 string               `json:"customerId"`
	OrgUnitPath                string               `json:"orgUnitPath"`
	IsMailboxSetup             bool                 `json:"isMailboxSetup"`
	IsEnrolledIn2Sv            bool                 `json:"isEnrolledIn2Sv"`
	IsEnforcedIn2Sv            bool                 `json:"isEnforcedIn2Sv"`
	IncludeInGlobalAddressList bool                 `json:"includeInGlobalAddressList"`
	ThumbnailPhotoURL          string               `json:"thumbnailPhotoUrl"`
	ThumbnailPhotoEtag         string               `json:"thumbnailPhotoEtag"`
	CustomSchemas              struct{}             `json:"customSchemas"`
}
