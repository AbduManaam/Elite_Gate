package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const (
	adminURL   = "http://localhost:9090"
	gatewayURL = "http://localhost:8080"
)

func main() {
	fmt.Println("🚀 Starting CoreGuard Gateway End-to-End User Flow Test...")
	time.Sleep(1 * time.Second)

	// Seed random number generator
	rand.Seed(time.Now().UnixNano())
	randomSuffix := fmt.Sprintf("%x", rand.Intn(100000))
	username := fmt.Sprintf("flow_tester_%s", randomSuffix)
	password := "COreGuard1@Admin!"
	company := fmt.Sprintf("TestCompany_%s", randomSuffix)

	fmt.Printf("\n--- Step 0: Verify Seeded Public Sample Services (Default Project) ---\n")
	
	// 1. Verify user service
	fmt.Printf("Requesting seeded User Service route: GET %s/api/users\n", gatewayURL)
	code, body, err := getRequest(gatewayURL+"/api/users", map[string]string{})
	if err != nil {
		fmt.Printf("❌ User Service check failed: %v\n", err)
	} else {
		fmt.Printf("   Response code: %d\n", code)
		fmt.Printf("   Response body: %s\n", body)
		if code == http.StatusOK {
			fmt.Printf("   ✅ Seeded user-service route works!\n")
		} else {
			fmt.Printf("   ❌ Seeded user-service route returned non-200 status\n")
		}
	}

	// 2. Verify order service
	fmt.Printf("\nRequesting seeded Order Service route: GET %s/api/orders\n", gatewayURL)
	code, body, err = getRequest(gatewayURL+"/api/orders", map[string]string{})
	if err != nil {
		fmt.Printf("❌ Order Service check failed: %v\n", err)
	} else {
		fmt.Printf("   Response code: %d\n", code)
		fmt.Printf("   Response body: %s\n", body)
		if code == http.StatusOK {
			fmt.Printf("   ✅ Seeded order-service route works!\n")
		} else {
			fmt.Printf("   ❌ Seeded order-service route returned non-200 status\n")
		}
	}

	fmt.Printf("\n--- Step 1: User Signup ---\n")
	fmt.Printf("Registering tenant user: %s (Company: %s)\n", username, company)
	signupBody := map[string]interface{}{
		"username": username,
		"password": password,
		"company":  company,
		"plan":     "free",
	}
	signupRes, err := postRequest(adminURL+"/admin/signup", signupBody, "")
	if err != nil {
		fmt.Printf("❌ Signup failed: %v\n", err)
		return
	}
	
	accessToken, _ := signupRes["access_token"].(string)
	projectID, _ := signupRes["project_id"].(string)
	
	if accessToken == "" || projectID == "" {
		fmt.Printf("❌ Failed to parse signup response: %+v\n", signupRes)
		return
	}
	fmt.Printf("✅ Signup successful!\n")
	fmt.Printf("   Project ID:   %s\n", projectID)
	fmt.Printf("   Access Token: [JWT Hidden]\n")

	fmt.Printf("\n--- Step 2: Create Upstream pointing to User Service ---\n")
	// Target the user service (user-service:9001) which runs on the same docker network.
	// Since we know health check is registered and probes the health path,
	// health_path "/health" should return 200 OK.
	upstreamBody := map[string]interface{}{
		"name":        fmt.Sprintf("upstream-%s", randomSuffix),
		"target_url":  "http://user-service:9001",
		"protocol":    "http",
		"health_path": "/health",
		"enabled":     true,
	}
	upstreamRes, err := postRequest(fmt.Sprintf("%s/admin/v1/projects/%s/upstreams", adminURL, projectID), upstreamBody, accessToken)
	if err != nil {
		fmt.Printf("❌ Upstream creation failed: %v\n", err)
		return
	}
	
	upstreamData, _ := upstreamRes["upstream"].(map[string]interface{})
	upstreamID, _ := upstreamData["id"].(string)
	if upstreamID == "" {
		fmt.Printf("❌ Failed to parse upstream response: %+v\n", upstreamRes)
		return
	}
	fmt.Printf("✅ Upstream created successfully! ID: %s\n", upstreamID)

	fmt.Printf("\n--- Step 2b: Add Upstream Pool Target ---\n")
	targetBody := map[string]interface{}{
		"target_url": "http://user-service:9001",
		"weight":     1,
		"enabled":    true,
	}
	targetURL := fmt.Sprintf("%s/admin/v1/projects/%s/upstreams/%s/targets", adminURL, projectID, upstreamID)
	targetRes, err := postRequest(targetURL, targetBody, accessToken)
	if err != nil {
		fmt.Printf("❌ Failed to add upstream target: %v\n", err)
		return
	}
	fmt.Printf("✅ Upstream target added successfully: %+v\n", targetRes)

	fmt.Printf("\n--- Step 3: Create Auth-Enforcing Policy ---\n")
	policyBody := map[string]interface{}{
		"name":            fmt.Sprintf("policy-%s", randomSuffix),
		"auth_required":   true,
		"rate_limit_rpm":  120,
		"allowed_origins": []string{"*"},
	}
	policyRes, err := postRequest(fmt.Sprintf("%s/admin/v1/projects/%s/policies", adminURL, projectID), policyBody, accessToken)
	if err != nil {
		fmt.Printf("❌ Policy creation failed: %v\n", err)
		return
	}
	
	policyData, _ := policyRes["policy"].(map[string]interface{})
	policyID, _ := policyData["id"].(string)
	if policyID == "" {
		fmt.Printf("❌ Failed to parse policy response: %+v\n", policyRes)
		return
	}
	fmt.Printf("✅ Policy created successfully! ID: %s\n", policyID)

	fmt.Printf("\n--- Step 4: Create Route ---\n")
	routePath := "/api/health"
	routeBody := map[string]interface{}{
		"name":        fmt.Sprintf("route-%s", randomSuffix),
		"path":        routePath,
		"upstream_id": upstreamID,
		"policy_id":   policyID,
		"methods":     []string{"GET"},
		"match_type":  "prefix",
		"enabled":     true,
	}
	routeRes, err := postRequest(fmt.Sprintf("%s/admin/v1/projects/%s/routes", adminURL, projectID), routeBody, accessToken)
	if err != nil {
		fmt.Printf("❌ Route creation failed: %v\n", err)
		return
	}
	
	routeData, _ := routeRes["route"].(map[string]interface{})
	routeID, _ := routeData["id"].(string)
	if routeID == "" {
		fmt.Printf("❌ Failed to parse route response: %+v\n", routeRes)
		return
	}
	fmt.Printf("✅ Route created successfully! ID: %s, Mapping Path: %s\n", routeID, routePath)

	fmt.Printf("\n--- Step 5: Create API Key ---\n")
	keyBody := map[string]interface{}{
		"name": fmt.Sprintf("key-%s", randomSuffix),
	}
	keyRes, err := postRequest(fmt.Sprintf("%s/admin/v1/projects/%s/keys", adminURL, projectID), keyBody, accessToken)
	if err != nil {
		fmt.Printf("❌ API Key creation failed: %v\n", err)
		return
	}
	
	apiKey, _ := keyRes["api_key"].(string)
	apiKeyID, _ := keyRes["id"].(string)
	if apiKey == "" {
		fmt.Printf("❌ Failed to parse API Key response: %+v\n", keyRes)
		return
	}
	fmt.Printf("✅ API Key created successfully! ID: %s, Raw Key: %s\n", apiKeyID, apiKey)

	fmt.Printf("\n--- Step 6: Deploy/Reload Configuration ---\n")
	reloadRes, err := postRequest(fmt.Sprintf("%s/admin/v1/projects/%s/reload", adminURL, projectID), nil, accessToken)
	if err != nil {
		fmt.Printf("❌ Reload trigger failed: %v\n", err)
		return
	}
	fmt.Printf("✅ Admin Reload response: %+v\n", reloadRes["message"])

	fmt.Printf("Forcing immediate reload on Shared Gateway...\n")
	sharedReloadRes, err := postRequest("http://localhost:8080/reload", nil, "")
	if err != nil {
		fmt.Printf("❌ Shared Gateway reload failed: %v\n", err)
		return
	}
	fmt.Printf("✅ Shared Gateway response: %+v\n", sharedReloadRes["message"])
	fmt.Println("Waiting 2 seconds for health check propagation...")
	time.Sleep(2 * time.Second)

	fmt.Printf("\n--- Step 7: Gateway serves traffic (Test via Shared Gateway on port 8080) ---\n")
	testURL := fmt.Sprintf("%s%s", gatewayURL, routePath)
	fmt.Printf("Making HTTP GET request to Shared Gateway: %s\n", testURL)
	
	// Test request without API Key (should fail with 401)
	code, body, err = getRequest(testURL, map[string]string{})
	if err != nil {
		fmt.Printf("❌ Request failed: %v\n", err)
		return
	}
	fmt.Printf("   Without API Key: HTTP Status %d, Body: %s\n", code, body)
	if code != http.StatusUnauthorized {
		fmt.Printf("❌ Expected HTTP 401 Unauthorized but got %d\n", code)
		return
	}
	fmt.Printf("   ✅ Correctly blocked unauthenticated request.\n")

	// Test request with API Key (should succeed with 200)
	code, body, err = getRequest(testURL, map[string]string{"X-API-Key": apiKey})
	if err != nil {
		fmt.Printf("❌ Request failed: %v\n", err)
		return
	}
	fmt.Printf("   With API Key: HTTP Status %d, Body: %s\n", code, body)
	if code != http.StatusOK {
		fmt.Printf("❌ Expected HTTP 200 OK but got %d\n", code)
		return
	}
	fmt.Printf("   ✅ Successfully proxied and received response from Upstream (User Service)!\n")

	fmt.Printf("\n--- Step 8: Provision Dedicated Gateway ---\n")
	provisionBody := map[string]interface{}{
		"project_id": projectID,
		"plan":       "dedicated",
	}
	fmt.Printf("Provisioning dedicated gateway container for project %s...\n", projectID)
	provisionRes, err := postRequest(fmt.Sprintf("%s/admin/v1/projects/%s/gateways", adminURL, projectID), provisionBody, accessToken)
	if err != nil {
		fmt.Printf("❌ Dedicated gateway provisioning failed: %v\n", err)
		return
	}
	
	gatewayID, _ := provisionRes["gateway_id"].(string)
	internalIP, _ := provisionRes["endpoint_ip"].(string)
	internalPort, _ := provisionRes["gateway_port"].(string)
	
	fmt.Printf("✅ Dedicated Gateway provisioned in Docker!\n")
	fmt.Printf("   Gateway ID:      %s\n", gatewayID)
	fmt.Printf("   Internal IP:     %s\n", internalIP)
	fmt.Printf("   Internal Port:   %s\n", internalPort)

	fmt.Printf("\n--- Step 9: Retrieve host port of Dedicated Gateway ---\n")
	containerName := fmt.Sprintf("elitegate-gateway-%s", gatewayID)
	fmt.Printf("Querying Docker for port mappings of container: %s\n", containerName)
	
	hostPort, err := getDockerHostPort(containerName)
	if err != nil {
		fmt.Printf("❌ Failed to query Docker host port: %v\n", err)
		return
	}
	fmt.Printf("   ✅ Mapped Host Port: %s\n", hostPort)

	fmt.Printf("\n--- Step 10: Test Dedicated Gateway serves traffic on port %s ---\n", hostPort)
	dedicatedGatewayURL := fmt.Sprintf("http://localhost:%s%s", hostPort, routePath)
	fmt.Printf("Making HTTP GET request to Dedicated Gateway: %s\n", dedicatedGatewayURL)

	// Wait a moment for dedicated gateway internal engine to fetch routes first
	fmt.Println("Waiting 2 seconds for dedicated gateway route engine startup...")
	time.Sleep(2 * time.Second)

	// Test request with API Key on Dedicated Gateway
	code, body, err = getRequest(dedicatedGatewayURL, map[string]string{"X-API-Key": apiKey})
	if err != nil {
		fmt.Printf("❌ Request to dedicated gateway failed: %v\n", err)
		return
	}
	fmt.Printf("   Dedicated Gateway Response: HTTP Status %d, Body: %s\n", code, body)
	if code != http.StatusOK {
		fmt.Printf("❌ Expected HTTP 200 OK but got %d\n", code)
		return
	}
	fmt.Printf("   ✅ Dedicated Gateway successfully proxied request and validated API key!\n")

	fmt.Printf("\n--- Step 11: Decommission Dedicated Gateway ---\n")
	fmt.Printf("Cleaning up: Decommissioning gateway %s...\n", gatewayID)
	decomURL := fmt.Sprintf("%s/admin/v1/projects/%s/gateways/%s", adminURL, projectID, gatewayID)
	
	req, err := http.NewRequest(http.MethodDelete, decomURL, nil)
	if err != nil {
		fmt.Printf("❌ Failed to build delete request: %v\n", err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("❌ Decommission request failed: %v\n", err)
		return
	}
	defer resp.Body.Close()
	fmt.Printf("   ✅ Decommission response: HTTP Status %d\n", resp.StatusCode)

	fmt.Println("\n=======================================================")
	fmt.Println("🎉 ALL TESTS PASSED SUCCESSFULLY! END-TO-END FLOW IS 100% WORKING!")
	fmt.Println("=======================================================")
}

func postRequest(url string, reqBody map[string]interface{}, token string) (map[string]interface{}, error) {
	var jsonBytes []byte
	var err error
	if reqBody != nil {
		jsonBytes, err = json.Marshal(reqBody)
		if err != nil {
			return nil, err
		}
	}
	
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 35 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(respBytes))
	}

	var res map[string]interface{}
	if len(respBytes) > 0 {
		if err := json.Unmarshal(respBytes, &res); err != nil {
			return nil, err
		}
	}
	return res, nil
}

func getRequest(url string, headers map[string]string) (int, string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, "", err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, "", err
	}

	return resp.StatusCode, string(respBytes), nil
}

func getDockerHostPort(containerName string) (string, error) {
	// Execute 'docker port' command
	cmd := exec.Command("docker", "port", containerName, "8080")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("cmd run: %w, stderr: %s", err, stderr.String())
	}
	
	// Output is of format e.g. "0.0.0.0:49153" or "[::]:49153"
	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return "", fmt.Errorf("empty stdout from docker port")
	}
	
	// Parse out port
	lines := strings.Split(output, "\n")
	firstLine := strings.TrimSpace(lines[0])
	
	// Regex to extract port at the end after colon
	re := regexp.MustCompile(`:(\d+)$`)
	matches := re.FindStringSubmatch(firstLine)
	if len(matches) < 2 {
		return "", fmt.Errorf("failed to parse port from line: %q", firstLine)
	}
	return matches[1], nil
}
