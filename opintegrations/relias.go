package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type ReliasEmployeeReport struct {
	LastName            string `csv:"Last Name"`
	FirstName           string `csv:"First Name"`
	Username            string `csv:"Username"`
	DateHired           string `csv:"Date Hired"`
	DateTerminated      string `csv:"Date Terminated"`
	Email               string `csv:"Email"`
	JobTitle            string `csv:"Default Jobs (HR)"`
	Department          string `csv:"Default Departments"`
	EmployeeType        string `csv:"Employee Type"`
	Location            string `csv:"Work Schedule"`
	ReliasAccountStatus string `csv:"Relias Account Status"`
}

func runAllReliasFeeds() {
	chs, err := lookupCompanyByShortname("CHS")
	if err != nil {
		ErrorLog.Println("err looking up company CHS, err: ", err)
		return
	}

	InfoLog.Println("Starting Relias for " + chs.ShortName)
	err = runReliasFeed(chs)
	if err != nil {
		ErrorLog.Println("RELIAS FAILED: " + err.Error())
	} else {
		InfoLog.Println("Relias feed successful")
	}
}

func runReliasFeed(company Company) error {
	reportID := 37754576

	cxn := chooseOPAPICxn(company.OPID)

	allEmployees := []ReliasEmployeeReport{}
	reportURL := fmt.Sprintf("https://secure.onehcm.com/ta/rest/v1/report/%s/%v?company:shortname=%s", "saved", reportID, company.ShortName)
	err := cxn.GenricReport(reportURL, &allEmployees)
	if err != nil {
		return errors.New("error getting relias report: " + err.Error())
	}

	dateToBeGreaterThan := time.Date(2018, 8, 29, 0, 0, 0, 0, time.UTC)
	opDateFormat := "01/02/2006"

	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return errors.New("could not load location")
	}
	now := time.Now().In(loc)
	creationFilename := now.Format("20060102150405")

	feed := ""
	for _, emp := range allEmployees {
		var termDate, hireDate *time.Time

		if emp.DateTerminated != "" {
			termDateV, _ := time.Parse(opDateFormat, emp.DateTerminated)
			termDate = &termDateV
		}

		if termDate != nil && (*termDate).Unix() < dateToBeGreaterThan.Unix() {
			continue
		}

		if emp.DateHired != "" {
			hireDateV, _ := time.Parse(opDateFormat, emp.DateHired)
			hireDate = &hireDateV
		}

		termDateReliasFormat, hireDateReliasFormat := "", ""
		if termDate != nil {
			termDateReliasFormat = termDate.Format("2006-01-02 15:04:05")
		}
		if hireDate != nil {
			hireDateReliasFormat = hireDate.Format("2006-01-02 15:04:05")
		}

		reliasAccountStatus := "1"
		if emp.ReliasAccountStatus == "OnLeave" {
			reliasAccountStatus = "2"
		}
		if emp.ReliasAccountStatus == "Inactive" || termDate != nil {
			reliasAccountStatus = "0"
		}

		format := "USER|12150|ASD12150|%s|%s|%s|welcome|||%s|%s|%s|%s|%s||%s||||||||||%s|||||||||||||%s|||||||||\r\n"
		line := fmt.Sprintf(format, emp.LastName, emp.FirstName, emp.Username,
			hireDateReliasFormat, termDateReliasFormat, emp.Email, emp.JobTitle,
			emp.Department, emp.Location, reliasAccountStatus, emp.EmployeeType)

		feed = feed + line
	}

	// local file testing
	// absPath, _ := filepath.Abs(fmt.Sprintf("./opintegrations/jobfiles/%s.txt", creationFilename))
	// file, err := os.Create(absPath)
	// if err != nil {
	// 	return err
	// }
	// defer file.Close()

	sshClient, sftpClient, err := connectSFTP(SFTPCredentials{"chsstk", "7jwQ+o[I", "sftp.reliaslearning.com"})
	if err != nil {
		return errors.New("connectSFTP err: " + err.Error())
	}

	file, err := sftpClient.Create(fmt.Sprintf("/%s.txt", creationFilename))
	if err != nil {
		return errors.New("sftpClient.Create err: " + err.Error())
	}
	defer file.Close()

	_, err = io.Copy(file, strings.NewReader(feed))
	if err != nil {
		return errors.New("sftpClient ioCopy err: " + err.Error())
	}

	sshClient.Close()
	sftpClient.Close()

	return nil
}

type SFTPCredentials struct {
	Username    string
	Password    string
	HostAddress string
}

func connectSFTP(creds SFTPCredentials) (*ssh.Client, *sftp.Client, error) {
	addr, err := net.LookupIP(creds.HostAddress)
	if err != nil {
		return nil, nil, errors.New("LookupIP err!: " + err.Error())
	}

	if len(addr) < 1 {
		return nil, nil, errors.New(fmt.Sprint("ip address was < 1: ", addr))
	}

	host := addr[0].String()

	// TODO: figure out HostKeyCallback
	// hostKey, err := getHostKey(host)
	// if err != nil {
	// 	return err
	// }

	config := &ssh.ClientConfig{
		User: creds.Username,
		Auth: []ssh.AuthMethod{
			ssh.Password(creds.Password),
		},
		// HostKeyCallback: ssh.FixedHostKey(hostKey),
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	conn, err := ssh.Dial("tcp", host+":22", config)
	if err != nil {
		return nil, nil, err
	}

	client, err := sftp.NewClient(conn)
	if err != nil {
		return nil, nil, err
	}

	return conn, client, nil
}

func getHostKey(host string) (ssh.PublicKey, error) {
	var hostKey ssh.PublicKey
	// parse OpenSSH known_hosts file
	// ssh or use ssh-keyscan to get initial key
	file, err := os.Open(filepath.Join(os.Getenv("HOME"), ".ssh", "known_hosts"))
	if err != nil {
		return hostKey, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), " ")
		if len(fields) != 3 {
			continue
		}
		if strings.Contains(fields[0], host) {
			var err error
			hostKey, _, _, _, err = ssh.ParseAuthorizedKey(scanner.Bytes())
			if err != nil {
				return hostKey, errors.New(fmt.Sprintf("error parsing %q: %v", fields[2], err))
			}
			break
		}
	}

	if hostKey == nil {
		return hostKey, errors.New(fmt.Sprintf("no hostkey found for %s", host))
	}

	return hostKey, nil
}
