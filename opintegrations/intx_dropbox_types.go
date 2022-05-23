package main

import "time"

type DropboxSuspendUserError struct {
	ErrorSummary string `json:"error_summary"`
	Error        struct {
		Tag string `json:".tag"`
	} `json:"error"`
}

type DropboxSuspendUserRequestBody struct {
	User     map[string]interface{} `json:"user"`
	WipeData bool                   `json:"wipe_data"`
}

type DropboxNewUser struct {
	MemberEmail      string `json:"member_email"`
	MemberGivenName  string `json:"member_given_name"`
	MemberSurname    string `json:"member_surname"`
	MemberExternalID string `json:"member_external_id"`
	SendWelcomeEmail bool   `json:"send_welcome_email"`
	Role             string `json:"role"`
}

type DropboxNewUserRequestBody struct {
	NewMembers []DropboxNewUser `json:"new_members"`
	ForceAsync bool             `json:"force_async"`
}

type DropboxNewUserResponseBody struct {
	Tag      string `json:".tag"`
	Complete []struct {
		Tag     string `json:".tag"`
		Profile struct {
			TeamMemberID  string `json:"team_member_id"`
			Email         string `json:"email"`
			EmailVerified bool   `json:"email_verified"`
			Status        struct {
				Tag string `json:".tag"`
			} `json:"status"`
			Name struct {
				GivenName       string `json:"given_name"`
				Surname         string `json:"surname"`
				FamiliarName    string `json:"familiar_name"`
				DisplayName     string `json:"display_name"`
				AbbreviatedName string `json:"abbreviated_name"`
			} `json:"name"`
			MembershipType struct {
				Tag string `json:".tag"`
			} `json:"membership_type"`
			Groups         []string  `json:"groups"`
			MemberFolderID string    `json:"member_folder_id"`
			ExternalID     string    `json:"external_id"`
			AccountID      string    `json:"account_id"`
			JoinedOn       time.Time `json:"joined_on"`
		} `json:"profile"`
		Role struct {
			Tag string `json:".tag"`
		} `json:"role"`
	} `json:"complete"`
}

type DropboxGetAdminUserResponse struct {
	AdminProfile DropboxTeamMemberProfile `json:"admin_profile"`
}

type DropboxTeamMemberProfile struct {
	TeamMemberID  string `json:"team_member_id"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Status        struct {
		Tag string `json:".tag"`
	} `json:"status"`
	Name struct {
		GivenName       string `json:"given_name"`
		Surname         string `json:"surname"`
		FamiliarName    string `json:"familiar_name"`
		DisplayName     string `json:"display_name"`
		AbbreviatedName string `json:"abbreviated_name"`
	} `json:"name"`
	MembershipType struct {
		Tag string `json:".tag"`
	} `json:"membership_type"`
	Groups         []string  `json:"groups"`
	MemberFolderID string    `json:"member_folder_id"`
	ExternalID     string    `json:"external_id"`
	AccountID      string    `json:"account_id"`
	JoinedOn       time.Time `json:"joined_on"`
}
