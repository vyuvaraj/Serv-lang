package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"servtunnel/pkg/client"
)

func runTunnelCmd() {
	tunnelCmd := flag.NewFlagSet("tunnel", flag.ExitOnError)
	relayFlag := tunnelCmd.String("relay", "ws://localhost:8443/ws/connect", "Relay server WebSocket URL")
	subdomainFlag := tunnelCmd.String("subdomain", "", "Requested subdomain")
	customDomainFlag := tunnelCmd.String("custom-domain", "", "Requested custom domain mapping")
	tokenFlag := tunnelCmd.String("token", "", "Authentication token")
	inspectPortFlag := tunnelCmd.String("inspect-port", "4040", "Local inspection web UI port (use 0 or empty to disable)")
	shareAuthFlag := tunnelCmd.String("share-auth", "", "Basic authentication credentials (usr:pwd) to protect the public tunnel")

	// Allow short flags as well
	tunnelCmd.StringVar(relayFlag, "r", "ws://localhost:8443/ws/connect", "Relay server WebSocket URL")
	tunnelCmd.StringVar(subdomainFlag, "s", "", "Requested subdomain")
	tunnelCmd.StringVar(customDomainFlag, "c", "", "Requested custom domain mapping")
	tunnelCmd.StringVar(tokenFlag, "t", "", "Authentication token")
	tunnelCmd.StringVar(inspectPortFlag, "i", "4040", "Local inspection web UI port")
	tunnelCmd.StringVar(shareAuthFlag, "a", "", "Basic authentication credentials (usr:pwd)")

	// Parse local port (mandatory argument)
	if len(os.Args) < 3 {
		fmt.Println("Usage: serv tunnel <local-port> [options]")
		fmt.Println("\nOptions:")
		tunnelCmd.PrintDefaults()
		os.Exit(1)
	}

	localPort := os.Args[2]
	// Parse remaining flags starting from os.Args[3:]
	if err := tunnelCmd.Parse(os.Args[3:]); err != nil {
		fmt.Printf("Error parsing arguments: %v\n", err)
		os.Exit(1)
	}

	subdomain := *subdomainFlag
	customDomain := *customDomainFlag

	// If no subdomain, try to get git branch subdomain
	if subdomain == "" {
		if gitSub := getGitBranchSubdomainForServ(); gitSub != "" {
			subdomain = gitSub
			log.Printf("No subdomain specified. Auto-detected Git branch: %s", subdomain)
		}
	}

	localAddr := "localhost:" + localPort
	c := client.NewClient(localAddr, *relayFlag, subdomain, customDomain, *tokenFlag, *inspectPortFlag, *shareAuthFlag)

	if err := c.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Tunnel error: %v\n", err)
		os.Exit(1)
	}
}

func getGitBranchSubdomainForServ() string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" || branch == "HEAD" {
		return ""
	}

	// Lowercase
	branch = strings.ToLower(branch)
	// Replace non-alphanumeric character sequences with a single hyphen
	reg := regexp.MustCompile(`[^a-z0-9]+`)
	sanitized := reg.ReplaceAllString(branch, "-")
	// Trim leading/trailing hyphens
	sanitized = strings.Trim(sanitized, "-")

	return sanitized
}
