package waf

import "gymbag/internal/config"

func defaultBlockRules() []config.CustomRule {
	return []config.CustomRule{
		{
			Name:   "default-block-sensitive-files",
			Action: config.RuleActionBlock,
			Paths: []string{
				"/.env",
				"/.env.*",
				"/.git/*",
				"/.git",
				"/.svn/*",
				"/.hg/*",
				"/.DS_Store",
				"/.aws/*",
				"/.ssh/*",
				"/docker-compose.yml",
				"/docker-compose.yaml",
				"*.bak",
				"*.sql",
				"*.sqlite",
			},
		},
		{
			Name:   "default-block-exploit-probes",
			Action: config.RuleActionBlock,
			Paths: []string{
				"/wp-admin",
				"/wp-admin/*",
				"/wp-login.php",
				"/xmlrpc.php",
				"/phpmyadmin*",
				"/vendor/phpunit*",
				"/cgi-bin/*",
				"/actuator/env*",
				"/actuator/heapdump*",
			},
		},
		{
			Name:   "default-block-vuln-scanners",
			Action: config.RuleActionBlock,
			UserAgents: []string{
				"sqlmap",
				"nikto",
				"nuclei",
				"masscan",
				"zgrab",
				"gobuster",
				"dirbuster",
				"ffuf",
				"wfuzz",
				"feroxbuster",
				"acunetix",
				"nessus",
				"openvas",
			},
		},
		{
			Name:   "default-block-injection-queries",
			Action: config.RuleActionBlock,
			Queries: []string{
				`~(?i)(union\s+select|<script|javascript:|/etc/passwd|information_schema|\.\./|cmd\.exe|/bin/sh)`,
			},
		},
	}
}

func defaultVerifyRules() []config.CustomRule {
	return []config.CustomRule{
		{
			Name:   "default-verify-automation",
			Action: config.RuleActionVerify,
			UserAgents: []string{
				"curl/",
				"wget/",
				"python-requests",
				"python-httpx",
				"go-http-client",
				"aiohttp/",
				"okhttp/",
				"axios/",
				"node-fetch",
				"scrapy",
				"headlesschrome",
				"playwright",
				"puppeteer",
				"selenium",
				"phantomjs",
			},
		},
		{
			Name:   "default-verify-missing-ua",
			Action: config.RuleActionVerify,
			Headers: []config.KeyValueMatch{
				{Name: "User-Agent", Pattern: ""},
			},
		},
	}
}

// DefaultSecurityRules returns Gymbag's built-in open-box WAF ruleset.
// Baseline block rules are always included; when includeVerifyRules is true,
// default proof-of-work verification rules for automated HTTP clients and missing User-Agents are appended.
func DefaultSecurityRules(includeVerifyRules bool) []config.CustomRule {
	rules := defaultBlockRules()
	if includeVerifyRules {
		rules = append(rules, defaultVerifyRules()...)
	}
	return rules
}
