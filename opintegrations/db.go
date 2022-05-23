package main

import (
	"database/sql"

	_ "github.com/go-sql-driver/mysql"
	"gopkg.in/gorp.v2"
)

const (
	ProdHost   = "127.0.0.1"
	ProdDbUser = "jaredshahbazian"

	LocalHost   = "127.0.0.1"
	LocalDbUser = "root"

	DbName = "opintx"
)

var dbmap *gorp.DbMap

func initDB() {
	host := LocalHost
	password := passwords.LOCAL_DB_PW
	user := LocalDbUser

	if env.Production {
		host = ProdHost
		password = passwords.PROD_DB_PW
		user = ProdDbUser
	}

	db, err := sql.Open("mysql", user+":"+password+"@tcp("+host+":3306)/"+DbName)
	if err != nil {
		panic("💥 DB OPEN FAILED: " + err.Error())
	}

	err = db.Ping()
	if err != nil {
		panic("💥 DB PING FAILED: " + err.Error())
	}

	InfoLog.Println("Connected to DB ", host)

	dbmap = &gorp.DbMap{Db: db, Dialect: gorp.MySQLDialect{Engine: "InnoDB", Encoding: "UTF8"}}

	dbmap.AddTableWithName(Company{}, "companies")
	dbmap.AddTableWithName(Integration{}, "integrations")
	dbmap.AddTableWithName(CompanyIntegration{}, "company_integrations")
	dbmap.AddTableWithName(OAuthTokens{}, "oauth_tokens")
	dbmap.AddTableWithName(HFEvent{}, "hf_events")
	dbmap.AddTableWithName(HFResult{}, "hf_results")
	dbmap.AddTableWithName(Product{}, "products")
	dbmap.AddTableWithName(CompanyProduct{}, "company_products")
	dbmap.AddTableWithName(CompanyProductAssignment{}, "company_product_assignments")
	dbmap.AddTableWithName(User{}, "users")
	dbmap.AddTableWithName(JobApplication{}, "job_applications")
	dbmap.AddTableWithName(IndeedSponsorship{}, "indeed_sponsorships")
	dbmap.AddTableWithName(CurrentJobReq{}, "current_job_reqs")
	dbmap.AddTableWithName(ZipRecruiterSponsorship{}, "ziprecruiter_sponsorships")
	dbmap.AddTableWithName(BackgroundCheck{}, "background_checks")
	dbmap.AddTableWithName(HFLastTimeFetchedChanges{}, "hf_last_fetched_changes")
	dbmap.AddTableWithName(AccessControl{}, "access_controls")
	dbmap.AddTableWithName(CPInstallation{}, "cp_installations")
	dbmap.AddTableWithName(CPEnrollment{}, "cp_enrollments")
	dbmap.AddTableWithName(CPEmployee{}, "cp_employees")
	dbmap.AddTableWithName(CPChange{}, "cp_changes")
	dbmap.AddTableWithName(GoogleHireAuthInfo{}, "google_hire_auth")
	dbmap.AddTableWithName(GoogleHireCreatedApplicants{}, "google_hire_created_applicants")
	dbmap.AddTableWithName(WebClockPunch{}, "webclock_punches")

	err = dbmap.CreateTablesIfNotExists()
	if err != nil {
		panic("💥 DB ADD TABLES FAILED")
	}

	go runExecs()
}

func runExecs() {
	dbmap.Exec("ALTER TABLE companies ADD COLUMN op_id BIGINT(20)")
	dbmap.Exec("ALTER TABLE companies ADD COLUMN name VARCHAR(255)")
	dbmap.Exec("ALTER TABLE companies ADD COLUMN token VARCHAR(255)")
	dbmap.Exec("CREATE UNIQUE INDEX shortnameUnique ON companies (short_name)")
	dbmap.Exec("CREATE UNIQUE INDEX tokenUnique ON companies (token)")
	dbmap.Exec("ALTER TABLE integrations ADD COLUMN redirect_url VARCHAR(255)")
	dbmap.Exec("ALTER TABLE integrations ADD COLUMN access_url VARCHAR(255)")
	dbmap.Exec("ALTER TABLE oauth_tokens CHANGE integration_id company_integration_id VARCHAR(255)")
	dbmap.Exec("ALTER TABLE hf_events ADD COLUMN email VARCHAR(255)")
	dbmap.Exec("ALTER TABLE hf_events MODIFY email VARCHAR(255) NOT NULL")
	dbmap.Exec("ALTER TABLE company_integrations ADD COLUMN authed_username VARCHAR(255)")
	dbmap.Exec("ALTER TABLE company_integrations DROP COLUMN access_token")
	dbmap.Exec("ALTER TABLE company_integrations DROP COLUMN refresh_token")
	dbmap.Exec("ALTER TABLE company_integrations ADD COLUMN enabled TINYINT(1)")
	dbmap.Exec("ALTER TABLE oauth_tokens ADD COLUMN active TINYINT(1) DEFAULT 1")
	dbmap.Exec("ALTER TABLE integrations MODIFY redirect_url VARCHAR(1024)")
	dbmap.Exec("ALTER TABLE oauth_tokens MODIFY refresh_token VARCHAR(1024)")
	dbmap.Exec("ALTER TABLE oauth_tokens MODIFY access_token VARCHAR(10000)")
	dbmap.Exec("ALTER TABLE company_integrations ADD COLUMN created BIGINT(20) DEFAULT 0")
	dbmap.Exec("ALTER TABLE products MODIFY description VARCHAR(1024)")
	dbmap.Exec("ALTER TABLE company_products DROP COLUMN company_integration_id")
	dbmap.Exec("ALTER TABLE company_products ADD COLUMN company_id BIGINT(20)")
	dbmap.Exec("ALTER TABLE products ADD COLUMN url VARCHAR(255)")
	dbmap.Exec("ALTER TABLE users ADD COLUMN email VARCHAR(255)")
	dbmap.Exec("ALTER TABLE company_products ADD COLUMN settings VARCHAR(3000) NOT NULL DEFAULT '{}'")
	dbmap.Exec("ALTER TABLE integrations ADD COLUMN default_settings VARCHAR(3000) NOT NULL DEFAULT '{}'")
	dbmap.Exec("ALTER TABLE products ADD COLUMN default_settings VARCHAR(3000) NOT NULL DEFAULT '{}'")
	dbmap.Exec("ALTER TABLE job_applications ADD COLUMN finish_email_sent TINYINT(1) DEFAULT 0")
	dbmap.Exec("ALTER TABLE indeed_sponsorships ADD COLUMN company_id BIGINT(20)")
	dbmap.Exec("CREATE INDEX lookupbyopid ON current_job_reqs (op_id)")
	dbmap.Exec("ALTER TABLE current_job_reqs ADD COLUMN company_op_id BIGINT(20)")
	dbmap.Exec("ALTER TABLE background_checks ADD COLUMN client_email VARCHAR(255)")
	dbmap.Exec("ALTER TABLE background_checks ADD COLUMN op_job_id BIGINT(20)")
	dbmap.Exec("ALTER TABLE background_checks MODIFY op_applicant_id VARCHAR(255)")
	dbmap.Exec("ALTER TABLE background_checks MODIFY op_job_id VARCHAR(255)")
	dbmap.Exec("ALTER TABLE background_checks ADD COLUMN finished_url VARCHAR(1000)")
	dbmap.Exec("ALTER TABLE ziprecruiter_sponsorships ADD COLUMN time_submitted BIGINT(20) DEFAULT 0")
	dbmap.Exec("ALTER TABLE indeed_sponsorships ADD COLUMN time_submitted BIGINT(20) DEFAULT 0")
	dbmap.Exec("ALTER TABLE job_applications MODIFY data TEXT")
	dbmap.Exec("ALTER TABLE company_integrations MODIFY settings TEXT")
	dbmap.Exec("ALTER TABLE job_applications ADD COLUMN resume_url VARCHAR(255)")
	dbmap.Exec("ALTER TABLE job_applications ADD COLUMN applied_on_op TINYINT(1) DEFAULT 0")
	dbmap.Exec("ALTER TABLE job_applications ADD COLUMN followup_email_sent TINYINT(1) DEFAULT 0")
	dbmap.Exec("ALTER TABLE job_applications ADD COLUMN user_reviewed TINYINT(1) NOT NULL DEFAULT 0")
	dbmap.Exec("ALTER TABLE users ADD COLUMN token VARCHAR(255)")
	dbmap.Exec("ALTER TABLE ziprecruiter_sponsorships ADD COLUMN user_id BIGINT(20)")
	dbmap.Exec("ALTER TABLE indeed_sponsorships ADD COLUMN user_id BIGINT(20)")
	dbmap.Exec("ALTER TABLE users ADD COLUMN is_company_admin TINYINT(1) DEFAULT 0")
	dbmap.Exec("ALTER TABLE users ADD COLUMN is_system_admin TINYINT(1) DEFAULT 0")
	dbmap.Exec("ALTER TABLE forms MODIFY data TEXT")
	dbmap.Exec("ALTER TABLE wotc_applicants ADD COLUMN wotc_employee_id BIGINT(20)")

	dbmap.Exec("ALTER TABLE cp_enrollments MODIFY data TEXT")
	dbmap.Exec("ALTER TABLE cp_changes MODIFY data TEXT")
	dbmap.Exec("CREATE INDEX sync_pull ON cp_changes (company_id, id)")

	dbmap.Exec("ALTER TABLE cp_employees ADD COLUMN cost_center_0 BIGINT(20) DEFAULT 0")
	dbmap.Exec("ALTER TABLE cp_employees ADD COLUMN cost_center_1 BIGINT(20) DEFAULT 0")
	dbmap.Exec("ALTER TABLE cp_employees ADD COLUMN cost_center_2 BIGINT(20) DEFAULT 0")
	dbmap.Exec("ALTER TABLE cp_employees ADD COLUMN cost_center_3 BIGINT(20) DEFAULT 0")
	dbmap.Exec("ALTER TABLE cp_employees ADD COLUMN cost_center_4 BIGINT(20) DEFAULT 0")
	dbmap.Exec("ALTER TABLE cp_employees ADD COLUMN middle_initial VARCHAR(5) DEFAULT ''")
	dbmap.Exec("ALTER TABLE cp_employees ADD COLUMN active TINYINT(1) DEFAULT 1")

	dbmap.Exec("ALTER TABLE google_hire_auth ADD COLUMN notification_registration_name VARCHAR(500) DEFAULT ''")

	dbmap.Exec("ALTER TABLE google_hire_created_applicants MODIFY gh_applicant_resource_name VARCHAR(500)")
	dbmap.Exec("ALTER TABLE indeed_sponsorships ADD COLUMN contact_name VARCHAR(255) DEFAULT ''")

	dbmap.Exec("ALTER TABLE google_hire_auth ADD COLUMN last_fetched BIGINT(20) DEFAULT 0")

	dbmap.Exec("ALTER TABLE company_products MODIFY settings TEXT")
	dbmap.Exec("ALTER TABLE products MODIFY default_settings TEXT")

	dbmap.Exec("ALTER TABLE job_applications ADD COLUMN op_applicant_id BIGINT(20) DEFAULT NULL")
}
