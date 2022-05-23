package main

import (
	"time"

	cache "github.com/patrickmn/go-cache"
)

var cash *cache.Cache

const (
	CACHENAME_COMPANY_EMPLOYEES    = "employees"
	CACHENAME_EMPLOYEE_DETAIL      = "empdetail"
	CACHENAME_COMPANY_COST_CENTERS = "costcenters"

	DEFAULT_CACHE_EXPIRATION = 20 * time.Minute
)

func initCache() {
	cash = cache.New(DEFAULT_CACHE_EXPIRATION, 10*time.Minute)
}
