package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"gymbag/internal/config"
	"gymbag/internal/host"
)

func runRulesetCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printRulesetUsage(stdout)
		return 0
	}
	sub := strings.ToLower(strings.TrimSpace(args[0]))
	switch sub {
	case "help", "-h", "--help":
		printRulesetUsage(stdout)
		return 0
	case "list", "ls":
		return runRulesetList(args[1:], stdout, stderr)
	case "add":
		return runRulesetAdd(args[1:], stdout, stderr)
	case "update":
		return runRulesetUpdate(args[1:], stdout, stderr)
	case "remove", "rm", "delete":
		return runRulesetRemove(args[1:], stdout, stderr)
	case "validate":
		return runRulesetValidate(args[1:], stdout, stderr)
	case "init", "create":
		return runRulesetInit(args[1:], stdout, stderr)
	case "rule", "rules":
		return runRulesetRuleCmd(args[1:], stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset: unknown subcommand %q\n\n", args[0])
		printRulesetUsage(stderr)
		return 2
	}
}

func runRulesetList(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gymbag ruleset list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var cfgPath, domain string
	fs.StringVar(&cfgPath, "c", "", "Path to config file")
	fs.StringVar(&cfgPath, "config", "", "Path to config file")
	fs.StringVar(&domain, "d", "", "Filter by site domain")
	fs.StringVar(&domain, "domain", "", "Filter by site domain")
	if err := parseInterspersedFlags(fs, args); err != nil {
		return 2
	}
	path, cfg, _, err := loadConfigForEdit(cfgPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset list: %v\n", err)
		return 1
	}

	if domain == "" {
		_, _ = fmt.Fprintf(stdout, "%s global defaults (%d rule_sources, %d remote_rules, %d inline rules):\n",
			path, len(cfg.Defaults.RuleSources), len(cfg.Defaults.RemoteRules), len(cfg.Defaults.Rules))
		for _, rs := range cfg.Defaults.RuleSources {
			_, _ = fmt.Fprintf(stdout, "  [global] %s (dynamic=%t, name=%q)\n", rs.Source, rs.Dynamic, rs.Name)
		}
		for _, u := range cfg.Defaults.RemoteRules {
			_, _ = fmt.Fprintf(stdout, "  [global remote_rules] %s\n", u)
		}
	}
	normDomain := host.Normalize(domain)
	for _, s := range cfg.Sites {
		if normDomain != "" && s.Domain != normDomain {
			continue
		}
		_, _ = fmt.Fprintf(stdout, "Site %s (%d rule_sources, %d remote_rules, %d inline rules):\n",
			s.Domain, len(s.WAF.RuleSources), len(s.WAF.RemoteRules), len(s.WAF.Rules))
		for _, rs := range s.WAF.RuleSources {
			_, _ = fmt.Fprintf(stdout, "  - %s (dynamic=%t, name=%q)\n", rs.Source, rs.Dynamic, rs.Name)
		}
		for _, u := range s.WAF.RemoteRules {
			_, _ = fmt.Fprintf(stdout, "  - [remote_rules] %s\n", u)
		}
	}
	return 0
}

func runRulesetAdd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gymbag ruleset add", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		cfgPath     string
		domain      string
		globalScope bool
		source      string
		name        string
		dynamic     bool
		static      bool
	)
	fs.StringVar(&cfgPath, "c", "", "Path to config file")
	fs.StringVar(&cfgPath, "config", "", "Path to config file")
	fs.StringVar(&domain, "d", "", "Target site domain (omit for global/all sites)")
	fs.StringVar(&domain, "domain", "", "Target site domain (omit for global/all sites)")
	fs.BoolVar(&globalScope, "global", false, "Attach to global defaults and all sites")
	fs.StringVar(&source, "source", "", "Rule file path, directory path, or http(s):// URL")
	fs.StringVar(&name, "name", "", "Optional friendly ruleset name")
	fs.BoolVar(&dynamic, "dynamic", false, "Auto-reload when rule file/folder changes or on refresh interval")
	fs.BoolVar(&static, "static", false, "Load once without dynamic auto-reload")
	if err := parseInterspersedFlags(fs, args); err != nil {
		return 2
	}
	if source == "" && fs.NArg() > 0 {
		source = fs.Arg(0)
	}
	if dynamic && static {
		_, _ = fmt.Fprintln(stderr, "ERROR gymbag ruleset add: --dynamic and --static cannot be used together")
		return 2
	}
	rs := config.RuleSource{
		Source:  source,
		Name:    name,
		Dynamic: dynamic,
	}
	if err := rs.Validate(); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset add: %v\n", err)
		return 2
	}

	path, cfg, format, err := loadConfigForEdit(cfgPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset add: %v\n", err)
		return 1
	}

	if !rs.IsURL() {
		resolved := config.ResolveRuleSourcePath(rs.Source, cfg.BaseDir)
		if _, stErr := os.Stat(resolved); stErr == nil {
			if _, _, _, _, loadErr := config.LoadRuleSourceFromDisk(rs.Source, cfg.BaseDir); loadErr != nil {
				_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset add: %v\n", loadErr)
				return 1
			}
		}
	}

	if domain != "" && !globalScope {
		target := host.Normalize(domain)
		found := false
		for i := range cfg.Sites {
			if cfg.Sites[i].Domain == target {
				cfg.Sites[i].WAF.RuleSources = upsertRuleSource(cfg.Sites[i].WAF.RuleSources, rs)
				found = true
				break
			}
		}
		if !found {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset add: site %q not found\n", target)
			return 1
		}
	} else {
		cfg.Defaults.RuleSources = upsertRuleSource(cfg.Defaults.RuleSources, rs)
		for i := range cfg.Sites {
			cfg.Sites[i].WAF.RuleSources = upsertRuleSource(cfg.Sites[i].WAF.RuleSources, rs)
		}
	}

	if err := saveConfigForEdit(path, cfg, format); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset add: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "Added ruleset %s (dynamic=%t) to %s\n", rs.Source, rs.Dynamic, path)
	return 0
}

func upsertRuleSource(list []config.RuleSource, rs config.RuleSource) []config.RuleSource {
	for i := range list {
		if list[i].Source == rs.Source || (rs.Name != "" && list[i].Name == rs.Name) {
			list[i] = rs
			return list
		}
	}
	return append(list, rs)
}

func runRulesetUpdate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gymbag ruleset update", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		cfgPath     string
		domain      string
		globalScope bool
		target      string
		newSource   string
		newName     string
		dynamic     bool
		static      bool
	)
	fs.StringVar(&cfgPath, "c", "", "Path to config file")
	fs.StringVar(&cfgPath, "config", "", "Path to config file")
	fs.StringVar(&domain, "d", "", "Target site domain (omit for global/all sites)")
	fs.StringVar(&domain, "domain", "", "Target site domain (omit for global/all sites)")
	fs.BoolVar(&globalScope, "global", false, "Update across global defaults and all sites")
	fs.StringVar(&target, "target", "", "Existing ruleset source or name to update")
	fs.StringVar(&newSource, "source", "", "Updated source path/dir/URL")
	fs.StringVar(&newName, "name", "", "Updated friendly name")
	fs.BoolVar(&dynamic, "dynamic", false, "Enable dynamic auto-reload")
	fs.BoolVar(&static, "static", false, "Disable dynamic auto-reload")
	if err := parseInterspersedFlags(fs, args); err != nil {
		return 2
	}
	if target == "" && fs.NArg() > 0 {
		target = fs.Arg(0)
	}
	if strings.TrimSpace(target) == "" {
		_, _ = fmt.Fprintln(stderr, "ERROR gymbag ruleset update: target source or name is required")
		return 2
	}
	if dynamic && static {
		_, _ = fmt.Fprintln(stderr, "ERROR gymbag ruleset update: --dynamic and --static cannot be used together")
		return 2
	}

	path, cfg, format, err := loadConfigForEdit(cfgPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset update: %v\n", err)
		return 1
	}

	updateSlice := func(list []config.RuleSource) (int, error) {
		count := 0
		for i := range list {
			if list[i].Source == target || (list[i].Name != "" && list[i].Name == target) {
				if newSource != "" {
					list[i].Source = newSource
				}
				if newName != "" {
					list[i].Name = newName
				}
				if dynamic {
					list[i].Dynamic = true
				} else if static {
					list[i].Dynamic = false
				}
				if err := list[i].Validate(); err != nil {
					return 0, err
				}
				count++
			}
		}
		return count, nil
	}

	totalUpdated := 0
	if domain != "" && !globalScope {
		norm := host.Normalize(domain)
		found := false
		for i := range cfg.Sites {
			if cfg.Sites[i].Domain == norm {
				found = true
				n, uErr := updateSlice(cfg.Sites[i].WAF.RuleSources)
				if uErr != nil {
					_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset update: %v\n", uErr)
					return 1
				}
				totalUpdated += n
				break
			}
		}
		if !found {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset update: site %q not found\n", norm)
			return 1
		}
	} else {
		n, uErr := updateSlice(cfg.Defaults.RuleSources)
		if uErr != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset update: %v\n", uErr)
			return 1
		}
		totalUpdated += n
		for i := range cfg.Sites {
			sn, sErr := updateSlice(cfg.Sites[i].WAF.RuleSources)
			if sErr != nil {
				_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset update: %v\n", sErr)
				return 1
			}
			totalUpdated += sn
		}
	}

	if totalUpdated == 0 {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset update: ruleset %q not found\n", target)
		return 1
	}
	if err := saveConfigForEdit(path, cfg, format); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset update: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "Updated ruleset %s in %s\n", target, path)
	return 0
}

func runRulesetRemove(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gymbag ruleset remove", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		cfgPath     string
		domain      string
		globalScope bool
		target      string
	)
	fs.StringVar(&cfgPath, "c", "", "Path to config file")
	fs.StringVar(&cfgPath, "config", "", "Path to config file")
	fs.StringVar(&domain, "d", "", "Target site domain (omit for global/all sites)")
	fs.StringVar(&domain, "domain", "", "Target site domain (omit for global/all sites)")
	fs.BoolVar(&globalScope, "global", false, "Remove from global defaults and all sites")
	fs.StringVar(&target, "source", "", "Ruleset source or name to remove")
	if err := parseInterspersedFlags(fs, args); err != nil {
		return 2
	}
	if target == "" && fs.NArg() > 0 {
		target = fs.Arg(0)
	}
	target = strings.TrimSpace(target)
	if target == "" {
		_, _ = fmt.Fprintln(stderr, "ERROR gymbag ruleset remove: ruleset source or name is required")
		return 2
	}

	path, cfg, format, err := loadConfigForEdit(cfgPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset remove: %v\n", err)
		return 1
	}

	removeFromWAF := func(w *config.SiteWAFConfig) int {
		removed := 0
		var nextRS []config.RuleSource
		for _, rs := range w.RuleSources {
			if rs.Source == target || (rs.Name != "" && rs.Name == target) {
				removed++
				continue
			}
			nextRS = append(nextRS, rs)
		}
		w.RuleSources = nextRS

		var nextRemote []string
		for _, u := range w.RemoteRules {
			if u == target {
				removed++
				continue
			}
			nextRemote = append(nextRemote, u)
		}
		w.RemoteRules = nextRemote
		return removed
	}

	totalRemoved := 0
	if domain != "" && !globalScope {
		norm := host.Normalize(domain)
		found := false
		for i := range cfg.Sites {
			if cfg.Sites[i].Domain == norm {
				found = true
				totalRemoved += removeFromWAF(&cfg.Sites[i].WAF)
				break
			}
		}
		if !found {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset remove: site %q not found\n", norm)
			return 1
		}
	} else {
		totalRemoved += removeFromWAF(&cfg.Defaults)
		for i := range cfg.Sites {
			totalRemoved += removeFromWAF(&cfg.Sites[i].WAF)
		}
	}

	if totalRemoved == 0 {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset remove: ruleset %q not found\n", target)
		return 1
	}
	if err := saveConfigForEdit(path, cfg, format); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset remove: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "Removed ruleset %s from %s\n", target, path)
	return 0
}

func runRulesetValidate(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, "Usage: gymbag ruleset validate <file-or-dir>")
		return 2
	}
	target := args[0]
	rules, files, kind, _, err := config.LoadRuleSourceFromDisk(target, "")
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "invalid ruleset %s: %v\n", target, err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "Valid %s ruleset (%d rule(s) across %d file(s)):\n", kind, len(rules), len(files))
	for _, f := range files {
		_, _ = fmt.Fprintf(stdout, "  - %s\n", f)
	}
	return 0
}

func runRulesetInit(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gymbag ruleset init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var outPath, format string
	var force bool
	fs.StringVar(&outPath, "o", "", "Output ruleset file path")
	fs.StringVar(&outPath, "output", "", "Output ruleset file path")
	fs.StringVar(&format, "format", "", "Format: rules or json (auto-detected from extension if omitted)")
	fs.BoolVar(&force, "f", false, "Overwrite existing file")
	fs.BoolVar(&force, "force", false, "Overwrite existing file")
	if err := parseInterspersedFlags(fs, args); err != nil {
		return 2
	}
	if outPath == "" && fs.NArg() > 0 {
		outPath = fs.Arg(0)
	}
	if outPath == "" {
		outPath = "rules/default.rules"
	}
	if !force {
		if _, err := os.Stat(outPath); err == nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset init: %s already exists (use --force to overwrite)\n", outPath)
			return 1
		}
	}

	starter := []config.CustomRule{
		{
			Name:       "block-bad-bots",
			Action:     config.RuleActionBlock,
			UserAgents: []string{"BadScraper"},
		},
		{
			Name:    "verify-auth-posts",
			Action:  config.RuleActionVerify,
			Methods: []string{"POST"},
			Paths:   []string{"/login*", "/api/auth/*"},
		},
	}

	useJSON := strings.EqualFold(format, "json") || (format == "" && strings.HasSuffix(strings.ToLower(outPath), ".json"))
	var data []byte
	if useJSON {
		var err error
		data, err = config.FormatRulesJSON(starter)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset init: %v\n", err)
			return 1
		}
	} else {
		data = []byte("# Gymbag ruleset file\n" + config.FormatRules(starter))
	}

	if err := writeFileAtomically(outPath, data, 0o644); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset init: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "Created starter ruleset %s (%d rules)\n", outPath, len(starter))
	return 0
}

func runRulesetRuleCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, "Usage: gymbag ruleset rule <list|add|update|remove> [flags]")
		return 2
	}
	sub := strings.ToLower(strings.TrimSpace(args[0]))
	fs := flag.NewFlagSet("gymbag ruleset rule "+sub, flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		ruleFile string
		cfgPath  string
		domain   string
		ruleName string
		index    int
		action   string
		ips      multiStringFlag
		uas      multiStringFlag
		paths    multiStringFlag
		queries  multiStringFlag
		methods  multiStringFlag
		headers  multiStringFlag
		cookies  multiStringFlag
	)
	index = -1
	fs.StringVar(&ruleFile, "file", "", "Dedicated ruleset file path (.rules or .json)")
	fs.StringVar(&cfgPath, "c", "", "Config file path (for inline rules when --file is omitted)")
	fs.StringVar(&cfgPath, "config", "", "Config file path")
	fs.StringVar(&domain, "d", "", "Site domain in config file (defaults to first site)")
	fs.StringVar(&domain, "domain", "", "Site domain in config file")
	fs.StringVar(&ruleName, "name", "", "Rule name")
	fs.IntVar(&index, "index", -1, "0-based rule index for update/remove")
	fs.StringVar(&action, "action", "", "Rule action: allow, verify, or block")
	fs.Var(&ips, "ip", "IP or CIDR (repeatable or comma-separated)")
	fs.Var(&uas, "ua", "User-Agent pattern (repeatable)")
	fs.Var(&paths, "path", "Path pattern (repeatable)")
	fs.Var(&queries, "query", "Query pattern (repeatable)")
	fs.Var(&methods, "method", "HTTP method (repeatable)")
	fs.Var(&headers, "header", "Header matcher Name=Pattern (repeatable)")
	fs.Var(&cookies, "cookie", "Cookie matcher Name=Pattern (repeatable)")
	if err := parseInterspersedFlags(fs, args[1:]); err != nil {
		return 2
	}

	var (
		rules    []config.CustomRule
		saveFunc func([]config.CustomRule) error
		target   string
	)

	if ruleFile != "" {
		target = ruleFile
		var isJSON bool
		if raw, err := os.ReadFile(ruleFile); err == nil {
			trimmed := bytes.TrimSpace(raw)
			isJSON = strings.HasSuffix(strings.ToLower(ruleFile), ".json") ||
				(len(trimmed) > 0 && (trimmed[0] == '[' || (trimmed[0] == '{' && json.Valid(trimmed))))
			parsed, pErr := config.ParseRemoteRules(raw)
			if pErr != nil {
				_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset rule %s: %v\n", sub, pErr)
				return 1
			}
			rules = parsed
		} else if !os.IsNotExist(err) || sub != "add" {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset rule %s: %v\n", sub, err)
			return 1
		} else {
			isJSON = strings.HasSuffix(strings.ToLower(ruleFile), ".json")
		}
		saveFunc = func(next []config.CustomRule) error {
			for i := range next {
				if err := next[i].Validate(); err != nil {
					return err
				}
			}
			var out []byte
			if isJSON {
				var err error
				out, err = config.FormatRulesJSON(next)
				if err != nil {
					return err
				}
			} else {
				out = []byte(config.FormatRules(next))
			}
			return writeFileAtomically(ruleFile, out, 0o644)
		}
	} else {
		path, cfg, format, err := loadConfigForEdit(cfgPath)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset rule %s: %v\n", sub, err)
			return 1
		}
		siteIdx := 0
		if domain != "" {
			norm := host.Normalize(domain)
			siteIdx = -1
			for i, s := range cfg.Sites {
				if s.Domain == norm {
					siteIdx = i
					break
				}
			}
			if siteIdx == -1 {
				_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset rule %s: site %q not found\n", sub, norm)
				return 1
			}
		}
		target = fmt.Sprintf("%s (%s)", path, cfg.Sites[siteIdx].Domain)
		rules = append([]config.CustomRule(nil), cfg.Sites[siteIdx].WAF.Rules...)
		saveFunc = func(next []config.CustomRule) error {
			cfg.Sites[siteIdx].WAF.Rules = next
			return saveConfigForEdit(path, cfg, format)
		}
	}

	switch sub {
	case "list", "ls":
		_, _ = fmt.Fprintf(stdout, "%s (%d rule(s)):\n", target, len(rules))
		for i, r := range rules {
			_, _ = fmt.Fprintf(stdout, "  [%d] action=%s name=%q ips=%v paths=%v uas=%v methods=%v\n",
				i, r.Action, r.Name, r.IPs, r.Paths, r.UserAgents, r.Methods)
		}
		return 0

	case "add":
		if action == "" {
			_, _ = fmt.Fprintln(stderr, "ERROR gymbag ruleset rule add: --action <allow|verify|block> is required")
			return 2
		}
		hdrMatches, err := parseKeyValueMatches(headers)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset rule add: %v\n", err)
			return 2
		}
		ckMatches, err := parseKeyValueMatches(cookies)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset rule add: %v\n", err)
			return 2
		}
		r := config.CustomRule{
			Name:       ruleName,
			Action:     config.RuleAction(action),
			IPs:        splitCommaFlags(ips),
			UserAgents: []string(uas),
			Paths:      splitCommaFlags(paths),
			Queries:    []string(queries),
			Methods:    splitCommaFlags(methods),
			Headers:    hdrMatches,
			Cookies:    ckMatches,
		}
		if err := r.Validate(); err != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset rule add: %v\n", err)
			return 1
		}
		rules = append(rules, r)
		if err := saveFunc(rules); err != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset rule add: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "Added rule to %s (total: %d)\n", target, len(rules))
		return 0

	case "update":
		targetIdx := findRuleIndex(rules, index, ruleName)
		if targetIdx < 0 || targetIdx >= len(rules) {
			_, _ = fmt.Fprintln(stderr, "ERROR gymbag ruleset rule update: matching rule not found (pass --index or --name)")
			return 1
		}
		r := rules[targetIdx].Clone()
		if action != "" {
			r.Action = config.RuleAction(action)
		}
		if len(ips) > 0 {
			r.IPs = splitCommaFlags(ips)
		}
		if len(uas) > 0 {
			r.UserAgents = []string(uas)
		}
		if len(paths) > 0 {
			r.Paths = splitCommaFlags(paths)
		}
		if len(queries) > 0 {
			r.Queries = []string(queries)
		}
		if len(methods) > 0 {
			r.Methods = splitCommaFlags(methods)
		}
		if len(headers) > 0 {
			hdrMatches, err := parseKeyValueMatches(headers)
			if err != nil {
				_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset rule update: %v\n", err)
				return 2
			}
			r.Headers = hdrMatches
		}
		if len(cookies) > 0 {
			ckMatches, err := parseKeyValueMatches(cookies)
			if err != nil {
				_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset rule update: %v\n", err)
				return 2
			}
			r.Cookies = ckMatches
		}
		if err := r.Validate(); err != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset rule update: %v\n", err)
			return 1
		}
		rules[targetIdx] = r
		if err := saveFunc(rules); err != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset rule update: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "Updated rule #%d in %s\n", targetIdx, target)
		return 0

	case "remove", "rm", "delete":
		targetIdx := findRuleIndex(rules, index, ruleName)
		if targetIdx < 0 || targetIdx >= len(rules) {
			_, _ = fmt.Fprintln(stderr, "ERROR gymbag ruleset rule remove: matching rule not found (pass --index or --name)")
			return 1
		}
		rules = append(rules[:targetIdx], rules[targetIdx+1:]...)
		if err := saveFunc(rules); err != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset rule remove: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "Removed rule #%d from %s (remaining: %d)\n", targetIdx, target, len(rules))
		return 0

	default:
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag ruleset rule: unknown subcommand %q (expected list, add, update, remove)\n", sub)
		return 2
	}
}

func findRuleIndex(rules []config.CustomRule, index int, name string) int {
	if index >= 0 && index < len(rules) {
		return index
	}
	if strings.TrimSpace(name) != "" {
		for i, r := range rules {
			if r.Name == name {
				return i
			}
		}
	}
	return -1
}

func parseKeyValueMatches(items []string) ([]config.KeyValueMatch, error) {
	if len(items) == 0 {
		return nil, nil
	}
	out := make([]config.KeyValueMatch, 0, len(items))
	for _, raw := range items {
		k, v, ok := strings.Cut(raw, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, fmt.Errorf("invalid matcher %q (expected Name=Pattern)", raw)
		}
		out = append(out, config.KeyValueMatch{
			Name:    strings.TrimSpace(k),
			Pattern: v,
		})
	}
	return out, nil
}

func printRulesetUsage(w io.Writer) {
	_, _ = fmt.Fprintf(w, `Usage:
  gymbag ruleset <subcommand> [flags]

Subcommands:
  list                   List rule sources configured in Gymbagfile / gymbag.json
  add                    Link a rule file, rules folder, or URL (supports --dynamic)
  update                 Update an existing ruleset's source, name, or --dynamic/--static mode
  remove                 Unlink a ruleset from Gymbagfile / gymbag.json
  validate <path>        Validate a dedicated rule file or rules directory
  init <path>            Create a starter ruleset file (.rules or .json)
  rule <cmd>             Manage individual rules inside --file or inline in config (list, add, update, remove)

Examples:
  gymbag ruleset init ./rules/custom.rules
  gymbag ruleset add --source ./rules --dynamic
  gymbag ruleset update ./rules --static --name corp-rules
  gymbag ruleset remove ./rules
  gymbag ruleset rule add --file ./rules/custom.rules --action block --name bad-bot --ua BadBot
  gymbag ruleset rule update --file ./rules/custom.rules --name bad-bot --action verify
  gymbag ruleset rule remove --file ./rules/custom.rules --name bad-bot
`)
}
