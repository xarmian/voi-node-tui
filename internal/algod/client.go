// Package algod provides connection resolution and a narrowly scoped admin client.
package algod

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	sdk "github.com/algorand/go-algorand-sdk/v2/client/v2/algod"
	"github.com/algorand/go-algorand-sdk/v2/client/v2/common/models"
	"github.com/algorand/go-algorand-sdk/v2/types"
)

// ParticipationKey is the REST model missing from the SDK's public client.
// Optional effective rounds use pointers so "not registered" differs from round 0.
type ParticipationKey struct {
	Id                  string                      `json:"id"`
	Address             string                      `json:"address"`
	Key                 models.AccountParticipation `json:"key"`
	EffectiveFirstValid *uint64                     `json:"effective-first-valid,omitempty"`
	EffectiveLastValid  *uint64                     `json:"effective-last-valid,omitempty"`
	LastVote            uint64                      `json:"last-vote,omitempty"`
	LastBlockProposal   uint64                      `json:"last-block-proposal,omitempty"`
	LastStateProof      uint64                      `json:"last-state-proof,omitempty"`
}
type Client struct {
	sdk      *sdk.Client
	http     *http.Client
	conn     Connection
	readOnly bool
}

// transport disallows redirects, including SDK requests, to keep the token on the
// configured origin. Successful response bodies never enter an error message.
type transport struct{ base http.RoundTripper }

func (t transport) RoundTrip(r *http.Request) (*http.Response, error) {
	res, err := t.base.RoundTrip(r)
	if err != nil {
		return nil, errors.New("algod connection failed")
	}
	if res.StatusCode >= 300 && res.StatusCode < 400 {
		res.Body.Close()
		return nil, errors.New("algod redirects are disabled")
	}
	return res, nil
}
func New(conn Connection, readOnly bool) (*Client, error) {
	checked, err := connection(conn.URL, conn.Token)
	if err != nil {
		return nil, err
	}
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = nil // Always connect directly; never send the admin token to a proxy.
	tr := transport{base: base}
	s, err := sdk.MakeClientWithTransport(checked.URL, checked.Token, nil, tr)
	if err != nil {
		return nil, errors.New("cannot initialize algod client")
	}
	return &Client{sdk: s, http: &http.Client{Transport: tr, Timeout: 70 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, conn: checked, readOnly: readOnly}, nil
}
func safeError(op string, err error) error {
	if err != nil {
		return fmt.Errorf("%s failed; check connection and admin-token permissions", op)
	}
	return nil
}
func (c *Client) Status(ctx context.Context) (models.NodeStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	s, err := c.sdk.Status().Do(ctx)
	return s, safeError("status", err)
}
func (c *Client) Wait(ctx context.Context, round uint64) (models.NodeStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, 65*time.Second)
	defer cancel()
	s, err := c.sdk.StatusAfterBlock(round).Do(ctx)
	return s, safeError("block wait", err)
}
func (c *Client) Versions(ctx context.Context) (models.Version, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	v, err := c.sdk.Versions().Do(ctx)
	return v, safeError("versions", err)
}

// BlockTime returns a block's header timestamp (unix seconds).
func (c *Client) BlockTime(ctx context.Context, round uint64) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	blk, err := c.sdk.Block(round).HeaderOnly(true).Do(ctx)
	return blk.TimeStamp, safeError("block header", err)
}
func (c *Client) Supply(ctx context.Context) (models.Supply, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	s, err := c.sdk.Supply().Do(ctx)
	return s, safeError("supply", err)
}
func (c *Client) Account(ctx context.Context, address string) (models.Account, error) {
	if _, err := types.DecodeAddress(address); err != nil {
		return models.Account{}, errors.New("invalid account checksum")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	a, err := c.sdk.AccountInformation(address).Exclude([]string{"all"}).Do(ctx)
	if err != nil {
		return a, safeError("account lookup", err)
	}
	if a.Address != address || (a.Status != "Online" && a.Status != "Offline" && a.Status != "NotParticipating") {
		return models.Account{}, errors.New("account lookup failed: incomplete or mismatched state")
	}
	return a, nil
}
func (c *Client) Keys(ctx context.Context) ([]ParticipationKey, error) {
	var keys []ParticipationKey
	err := c.request(ctx, http.MethodGet, "/v2/participation", &keys)
	return keys, err
}
func (c *Client) Generate(ctx context.Context, address string, first, last uint64) error {
	if c.readOnly {
		return errors.New("read-only: generation disabled")
	}
	if _, err := types.DecodeAddress(address); err != nil {
		return errors.New("invalid account checksum")
	}
	if last <= first {
		return errors.New("invalid generation range")
	}
	return c.request(ctx, http.MethodPost, fmt.Sprintf("/v2/participation/generate/%s?first=%d&last=%d", address, first, last), nil)
}
func (c *Client) Delete(ctx context.Context, id string) error {
	if c.readOnly {
		return errors.New("read-only: deletion disabled")
	}
	if id == "" {
		return errors.New("no key selected")
	}
	return c.request(ctx, http.MethodDelete, "/v2/participation/"+url.PathEscape(id), nil)
}
func (c *Client) request(ctx context.Context, method, path string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	r, err := http.NewRequestWithContext(ctx, method, c.conn.URL+path, nil)
	if err != nil {
		return errors.New("cannot create algod request")
	}
	r.Header.Set("X-Algo-API-Token", c.conn.Token)
	res, err := c.http.Do(r)
	if err != nil {
		return safeError("participation request", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("participation request failed (HTTP %d)", res.StatusCode)
	}
	if out == nil {
		_, err = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
		return safeError("participation response", err)
	}
	if err = json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(out); err != nil {
		return errors.New("invalid participation response")
	}
	return nil
}
