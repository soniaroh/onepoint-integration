package main

type SFErrorResponse []struct {
	Message   string        `json:"message"`
	ErrorCode string        `json:"errorCode"`
	Fields    []interface{} `json:"fields"`
}

type SFQueryResponse struct {
	Records []map[string]interface{} `json:"records"`
}

type SFCreateUserResponse struct {
	ID      string   `json:"id"`
	Errors  []string `json:"errors"`
	Success bool     `json:"success"`
}

type SFCreateUserRequest struct {
	Email             string `json:"Email"`
	FirstName         string `json:"FirstName"`
	LastName          string `json:"LastName"`
	Username          string `json:"Username"`
	Alias             string `json:"Alias"`
	CommunityNickname string `json:"CommunityNickname,omitempty"`
	TimeZoneSidKey    string `json:"TimeZoneSidKey"`
	LocaleSidKey      string `json:"LocaleSidKey"`
	EmailEncodingKey  string `json:"EmailEncodingKey"`
	ProfileId         string `json:"ProfileId"`
	LanguageLocaleKey string `json:"LanguageLocaleKey"`
	IsActive          bool   `json:"IsActive"`
	City              string `json:"City,omitempty"`
	Country           string `json:"Country,omitempty"`
	State             string `json:"State,omitempty"`
	Street            string `json:"Street,omitempty"`
	EmployeeNumber    string `json:"EmployeeNumber,omitempty"`
	Title             string `json:"Title,omitempty"`
	CompanyName       string `json:"CompanyName,omitempty"`
}

type SFUserInfoResponse struct {
	ID             string `json:"id"`
	AssertedUser   bool   `json:"asserted_user"`
	UserID         string `json:"user_id"`
	OrganizationID string `json:"organization_id"`
	Username       string `json:"username"`
	NickName       string `json:"nick_name"`
	DisplayName    string `json:"display_name"`
	Email          string `json:"email"`
	EmailVerified  bool   `json:"email_verified"`
	FirstName      string `json:"first_name"`
	LastName       string `json:"last_name"`
	Timezone       string `json:"timezone"`
	Photos         struct {
		Picture   string `json:"picture"`
		Thumbnail string `json:"thumbnail"`
	} `json:"photos"`
	AddrStreet           interface{} `json:"addr_street"`
	AddrCity             interface{} `json:"addr_city"`
	AddrState            interface{} `json:"addr_state"`
	AddrCountry          interface{} `json:"addr_country"`
	AddrZip              interface{} `json:"addr_zip"`
	MobilePhone          interface{} `json:"mobile_phone"`
	MobilePhoneVerified  bool        `json:"mobile_phone_verified"`
	IsLightningLoginUser bool        `json:"is_lightning_login_user"`
	Status               struct {
		CreatedDate interface{} `json:"created_date"`
		Body        interface{} `json:"body"`
	} `json:"status"`
	Urls struct {
		Enterprise   string `json:"enterprise"`
		Metadata     string `json:"metadata"`
		Partner      string `json:"partner"`
		Rest         string `json:"rest"`
		Sobjects     string `json:"sobjects"`
		Search       string `json:"search"`
		Query        string `json:"query"`
		Recent       string `json:"recent"`
		ToolingSoap  string `json:"tooling_soap"`
		ToolingRest  string `json:"tooling_rest"`
		Profile      string `json:"profile"`
		Feeds        string `json:"feeds"`
		Groups       string `json:"groups"`
		Users        string `json:"users"`
		FeedItems    string `json:"feed_items"`
		FeedElements string `json:"feed_elements"`
		CustomDomain string `json:"custom_domain"`
	} `json:"urls"`
	Active           bool   `json:"active"`
	UserType         string `json:"user_type"`
	Language         string `json:"language"`
	Locale           string `json:"locale"`
	UtcOffset        int    `json:"utcOffset"`
	LastModifiedDate string `json:"last_modified_date"`
}
