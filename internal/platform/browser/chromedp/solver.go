package chromedp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/storage"
	"github.com/chromedp/chromedp"
)

const (
	defaultSolverEndpoint  = "http://127.0.0.1:8191/v1"
	solverMaxTimeout       = 60_000
	solverHTTPTimeout      = 65 * time.Second
	maxSolverResponseBytes = 64 << 20
	maxAssetBytes          = 32 << 20
)

func (p *Provider) FetchAsset(ctx context.Context, profileURL, assetURL, referer string) ([]byte, string, error) {
	profileKey, err := domainProfileKey(profileURL)
	if err != nil {
		return nil, "", err
	}
	state := p.getProfileState(profileKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return nil, "", err
	}
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	if state.userAgent != "" {
		req.Header.Set("User-Agent", state.userAgent)
	}
	target, _ := url.Parse(assetURL)
	for _, cookie := range state.cookies {
		domain := strings.TrimPrefix(strings.ToLower(cookie.Domain), ".")
		if domain != "" && target != nil && target.Hostname() != domain && !strings.HasSuffix(target.Hostname(), "."+domain) {
			continue
		}
		req.AddCookie(&http.Cookie{Name: cookie.Name, Value: cookie.Value})
	}
	resp, err := (&http.Client{Timeout: solverHTTPTimeout}).Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("asset returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAssetBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(body) > maxAssetBytes {
		return nil, "", errors.New("asset exceeds 32 MiB")
	}
	contentType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if contentType == "" {
		contentType = http.DetectContentType(body)
	}
	return body, contentType, nil
}

type solverRequest struct {
	Command       string         `json:"cmd"`
	URL           string         `json:"url"`
	MaxTimeout    int            `json:"maxTimeout"`
	WaitInSeconds int            `json:"waitInSeconds"`
	Cookies       []solverCookie `json:"cookies,omitempty"`
}

type solverResponse struct {
	Status   string         `json:"status"`
	Message  string         `json:"message"`
	Solution solverSolution `json:"solution"`
}

type solverSolution struct {
	URL       string         `json:"url"`
	Status    int            `json:"status"`
	Response  string         `json:"response"`
	Cookies   []solverCookie `json:"cookies"`
	UserAgent string         `json:"userAgent"`
}

type solverCookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain,omitempty"`
	Path     string  `json:"path,omitempty"`
	Expires  float64 `json:"expires,omitempty"`
	Expiry   float64 `json:"expiry,omitempty"`
	HTTPOnly bool    `json:"httpOnly,omitempty"`
	Secure   bool    `json:"secure,omitempty"`
	SameSite string  `json:"sameSite,omitempty"`
}

func fetchSolverSolution(ctx context.Context, solverEndpoint, targetURL string, cookies []solverCookie) (*solverSolution, error) {
	payload, err := json.Marshal(solverRequest{
		Command:       "request.get",
		URL:           targetURL,
		MaxTimeout:    solverMaxTimeout,
		WaitInSeconds: 5,
		Cookies:       cookies,
	})
	if err != nil {
		return nil, fmt.Errorf("chromedp solver: encode request: %w", err)
	}

	requestCtx, cancel := context.WithTimeout(ctx, solverHTTPTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, solverEndpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("chromedp solver: create request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := (&http.Client{Timeout: solverHTTPTimeout}).Do(request)
	if err != nil {
		return nil, fmt.Errorf("chromedp solver: request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("chromedp solver: unexpected HTTP status %d", response.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxSolverResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("chromedp solver: read response: %w", err)
	}
	if len(body) > maxSolverResponseBytes {
		return nil, errors.New("chromedp solver: response exceeds 64 MiB")
	}

	var result solverResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("chromedp solver: decode response: %w", err)
	}
	if result.Solution.Status != http.StatusOK {
		message := strings.TrimSpace(result.Message)
		if message == "" {
			message = result.Status
		}
		return nil, fmt.Errorf("chromedp solver: solution status %d: %s", result.Solution.Status, message)
	}
	if strings.TrimSpace(result.Solution.Response) == "" {
		return nil, errors.New("chromedp solver: solution response is empty")
	}
	return &result.Solution, nil
}

func applySolverSolution(ctx context.Context, targetURL string, solution *solverSolution) error {
	documentURL := strings.TrimSpace(solution.URL)
	if documentURL == "" {
		documentURL = targetURL
	}
	if err := applyCookiesToChromium(ctx, targetURL, solution.Cookies, solution.UserAgent); err != nil {
		return err
	}
	actions := make([]chromedp.Action, 0, 2)

	actions = append(actions, chromedp.ActionFunc(func(actionCtx context.Context) error {
		frameTree, err := page.GetFrameTree().Do(actionCtx)
		if err != nil {
			return err
		}
		return page.SetDocumentContent(frameTree.Frame.ID, solution.Response).Do(actionCtx)
	}))
	actions = append(actions, chromedp.Evaluate(fmt.Sprintf(`(() => {
		let base = document.querySelector("base[data-shiori-solver]");
		if (!base) {
			base = document.createElement("base");
			base.dataset.shioriSolver = "true";
			document.head.prepend(base);
		}
		base.href = %q;
	})()`, documentURL), nil))

	if err := chromedp.Run(ctx, actions...); err != nil {
		return fmt.Errorf("chromedp solver: apply solution: %w", err)
	}
	return nil
}

func applyCookiesToChromium(ctx context.Context, targetURL string, cookies []solverCookie, userAgent string) error {
	if len(cookies) == 0 && strings.TrimSpace(userAgent) == "" {
		return nil
	}
	actions := make([]chromedp.Action, 0, len(cookies)+1)
	if strings.TrimSpace(userAgent) != "" {
		actions = append(actions, emulation.SetUserAgentOverride(userAgent))
	}
	for _, cookie := range cookies {
		if strings.TrimSpace(cookie.Name) == "" {
			continue
		}
		params := network.SetCookie(cookie.Name, cookie.Value).
			WithURL(targetURL).
			WithSecure(cookie.Secure).
			WithHTTPOnly(cookie.HTTPOnly)
		if cookie.Domain != "" {
			params = params.WithDomain(cookie.Domain)
		}
		if cookie.Path != "" {
			params = params.WithPath(cookie.Path)
		}
		if sameSite := solverCookieSameSite(cookie.SameSite); sameSite != "" {
			params = params.WithSameSite(sameSite)
		}
		expires := cookie.Expires
		if expires == 0 {
			expires = cookie.Expiry
		}
		if expires > 0 {
			expiresAt := cdp.TimeSinceEpoch(time.Unix(int64(expires), 0))
			params = params.WithExpires(&expiresAt)
		}
		actions = append(actions, params)
	}

	if err := chromedp.Run(ctx, actions...); err != nil {
		return fmt.Errorf("chromedp solver: apply cookies: %w", err)
	}
	return nil
}

func captureChromiumState(ctx context.Context) ([]solverCookie, string, error) {
	browserCookies, err := storage.GetCookies().Do(ctx)
	if err != nil {
		return nil, "", err
	}
	cookies := make([]solverCookie, 0, len(browserCookies))
	for _, cookie := range browserCookies {
		if cookie == nil || strings.TrimSpace(cookie.Name) == "" {
			continue
		}
		cookies = append(cookies, solverCookie{
			Name:     cookie.Name,
			Value:    cookie.Value,
			Domain:   cookie.Domain,
			Path:     cookie.Path,
			Expires:  cookie.Expires,
			HTTPOnly: cookie.HTTPOnly,
			Secure:   cookie.Secure,
			SameSite: cookie.SameSite.String(),
		})
	}
	var userAgent string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`navigator.userAgent`, &userAgent)); err != nil {
		return nil, "", err
	}
	return cookies, userAgent, nil
}

func mergeSolverCookies(existing, incoming []solverCookie) []solverCookie {
	merged := make(map[string]solverCookie, len(existing)+len(incoming))
	add := func(cookie solverCookie) {
		if strings.TrimSpace(cookie.Name) == "" {
			return
		}
		key := strings.ToLower(cookie.Domain) + "\x00" + cookie.Path + "\x00" + cookie.Name
		expires := cookie.Expires
		if expires == 0 {
			expires = cookie.Expiry
		}
		if expires > 0 && expires <= float64(time.Now().Unix()) {
			delete(merged, key)
			return
		}
		merged[key] = cookie
	}
	for _, cookie := range existing {
		add(cookie)
	}
	for _, cookie := range incoming {
		add(cookie)
	}
	result := make([]solverCookie, 0, len(merged))
	for _, cookie := range merged {
		result = append(result, cookie)
	}
	return result
}

func solverCookieSameSite(value string) network.CookieSameSite {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "strict":
		return network.CookieSameSiteStrict
	case "lax":
		return network.CookieSameSiteLax
	case "none", "no_restriction":
		return network.CookieSameSiteNone
	default:
		return ""
	}
}
