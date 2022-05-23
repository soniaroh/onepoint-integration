package main

import (
	cron "gopkg.in/robfig/cron.v2"
)

func doNow() {
	// hiredFiredPull()
	runBgChecks()
	runIndeedFeed()
	// runUserProvisioningChanges()
	refreshTimeClockProductCaches()
}

func startCrons() {
	if env.Production {
		go doNow()
	}

	c := cron.New()

	c.AddFunc("TZ=America/Los_Angeles 0 09 * * *", func() {
		runApplyFollowUps()
	})

	c.AddFunc("TZ=America/Los_Angeles 30 18 * * *", func() {
		runAllReliasFeeds()
	})

	c.AddFunc("TZ=America/Los_Angeles 0 19 * * *", func() {
		runApplyFollowUps()
	})

	c.AddFunc("@every 25m", func() {
		runIndeedFeed()
	})

	c.AddFunc("@every 10m", func() {
		runBgChecks()
	})

	// c.AddFunc("@every 1m", func() {
	// 	hiredFiredPull()
	// })

	// c.AddFunc("@every 10m", func() {
	// 	runUserProvisioningChanges()
	// })

	c.AddFunc("@every 10m", func() {
		checkHFConnectionStatuses()
	})

	c.AddFunc("@every 30m", func() {
		refreshTimeClockProductCaches()
	})

	c.AddFunc("TZ=America/Los_Angeles 0 19 * * *", func() {
		renewGoogleHireRegistrations()
	})

	InfoLog.Println("starting crons")

	c.Start()
}
