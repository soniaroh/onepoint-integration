package main

type BoxErrorResponse struct {
	Type      string `json:"type"`
	Status    int    `json:"status"`
	Code      string `json:"code"`
	HelpURL   string `json:"help_url"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

type BoxUserSearchResponse struct {
	TotalCount int       `json:"total_count"`
	Entries    []BoxUser `json:"entries"`
	Limit      int       `json:"limit"`
	Offset     int       `json:"offset"`
}

type BoxUser struct {
	Type                          string        `json:"type"`
	ID                            string        `json:"id"`
	Name                          string        `json:"name"`
	Login                         string        `json:"login"`
	CreatedAt                     string        `json:"created_at"`
	ModifiedAt                    string        `json:"modified_at"`
	Role                          string        `json:"role"`
	Language                      string        `json:"language"`
	Timezone                      string        `json:"timezone"`
	TrackingCodes                 []interface{} `json:"tracking_codes"`
	CanSeeManagedUsers            bool          `json:"can_see_managed_users"`
	IsSyncEnabled                 bool          `json:"is_sync_enabled"`
	Status                        string        `json:"status"`
	JobTitle                      string        `json:"job_title"`
	Phone                         string        `json:"phone"`
	Address                       string        `json:"address"`
	AvatarURL                     string        `json:"avatar_url"`
	IsExemptFromDeviceLimits      bool          `json:"is_exempt_from_device_limits"`
	IsExemptFromLoginVerification bool          `json:"is_exempt_from_login_verification"`
	Enterprise                    struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"enterprise"`
	MyTags []string `json:"my_tags"`
}

type BoxCreateUserBody struct {
	Name        string `json:"name"`
	Login       string `json:"login"`
	JobTitle    string `json:"job_title,omitempty"`
	Phone       string `json:"phone,omitempty"`
	Address     string `json:"address,omitempty"`
	SpaceAmount int64  `json:"space_amount,omitempty"`
	Status      string `json:"status,omitempty"`
}
