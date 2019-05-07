package mention

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/datastore"
	"google.golang.org/api/iterator"
	"willnorris.com/go/microformats"
	"willnorris.com/go/webmention"

	"github.com/jcgregorio/slog"
	"github.com/jcgregorio/webmention-run/ds"
	"github.com/nfnt/resize"
)

const (
	MENTIONS         ds.Kind = "Mentions"
	WEB_MENTION_SENT ds.Kind = "WebMentionSent"
	THUMBNAIL        ds.Kind = "Thumbnail"
)

func (m *Mentions) close(c io.Closer) {
	if err := c.Close(); err != nil {
		m.log.Warningf("Failed to close: %s", err)
	}
}

type Mentions struct {
	DS  *ds.DS
	log slog.Logger
}

func NewMentions(ctx context.Context, project, ns string, log slog.Logger) (*Mentions, error) {
	d, err := ds.New(ctx, project, ns)
	if err != nil {
		return nil, err
	}
	return &Mentions{
		DS:  d,
		log: log,
	}, nil
}

type WebMentionSent struct {
	TS time.Time
}

func (m *Mentions) sent(source string) (time.Time, bool) {
	key := m.DS.NewKey(WEB_MENTION_SENT)
	key.Name = source

	dst := &WebMentionSent{}
	if err := m.DS.Client.Get(context.Background(), key, dst); err != nil {
		m.log.Warningf("Failed to find source: %q", source)
		return time.Time{}, false
	} else {
		m.log.Infof("Found source: %q", source)
		return dst.TS, true
	}
}

func (m *Mentions) recordSent(source string, updated time.Time) error {
	key := m.DS.NewKey(WEB_MENTION_SENT)
	key.Name = source

	src := &WebMentionSent{
		TS: updated.UTC(),
	}
	_, err := m.DS.Client.Put(context.Background(), key, src)
	return err
}

const (
	GOOD_STATE      = "good"
	UNTRIAGED_STATE = "untriaged"
	SPAM_STATE      = "spam"
)

type Mention struct {
	Source string
	Target string
	State  string
	TS     time.Time

	// Metadata found when validating. We might display this.
	Title     string    `datastore:",noindex"`
	Author    string    `datastore:",noindex"`
	AuthorURL string    `datastore:",noindex"`
	Published time.Time `datastore:",noindex"`
	Thumbnail string    `datastore:",noindex"`
}

func New(source, target string) *Mention {
	return &Mention{
		Source: source,
		Target: target,
		State:  UNTRIAGED_STATE,
		TS:     time.Now(),
	}
}

func (m *Mention) key() string {
	return fmt.Sprintf("%x", md5.Sum([]byte(m.Source+m.Target)))
}

func (m *Mention) FastValidate() error {
	if m.Source == "" {
		return fmt.Errorf("Source is empty.")
	}
	if m.Target == "" {
		return fmt.Errorf("Target is empty.")
	}
	if m.Target == m.Source {
		return fmt.Errorf("Source and Target must be different.")
	}
	target, err := url.Parse(m.Target)
	if err != nil {
		return fmt.Errorf("Target is not a valid URL: %s", err)
	}
	if target.Hostname() != "bitworking.org" {
		return fmt.Errorf("Wrong target domain.")
	}
	if target.Scheme != "https" {
		return fmt.Errorf("Wrong scheme for target.")
	}
	return nil
}

func (m *Mentions) SlowValidate(mention *Mention, c *http.Client) error {
	m.log.Infof("SlowValidate: %q", mention.Source)
	resp, err := c.Get(mention.Source)
	if err != nil {
		return fmt.Errorf("Failed to retrieve source: %s", err)
	}
	defer m.close(resp.Body)
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("Failed to read content: %s", err)
	}
	reader := bytes.NewReader(b)
	links, err := webmention.DiscoverLinksFromReader(reader, mention.Source, "")
	if err != nil {
		return fmt.Errorf("Failed to discover links: %s", err)
	}
	for _, link := range links {
		if link == mention.Target {
			_, err := reader.Seek(0, io.SeekStart)
			if err != nil {
				return nil
			}
			m.ParseMicroformats(mention, reader, MakeUrlToImageReader(c))
			return nil
		}
	}
	return fmt.Errorf("Failed to find target link in source.")
}

func (m *Mentions) ParseMicroformats(mention *Mention, r io.Reader, urlToImageReader UrlToImageReader) {
	u, err := url.Parse(mention.Source)
	if err != nil {
		return
	}
	data := microformats.Parse(r, u)
	m.findHEntry(context.Background(), urlToImageReader, mention, data, data.Items)
}

func (m *Mentions) VerifyQueuedMentions(c *http.Client) {
	queued := m.GetQueued(context.Background())
	m.log.Infof("About to slow verify %d queud mentions.", len(queued))
	for _, mention := range queued {
		mention.Published = time.Now()
		m.log.Infof("Verifying queued webmention from %q", mention.Source)
		if m.SlowValidate(mention, c) == nil {
			mention.State = GOOD_STATE
		} else {
			mention.State = SPAM_STATE
			m.log.Infof("Failed to validate webmention: %#v", *mention)
		}
		if err := m.Put(context.Background(), mention); err != nil {
			m.log.Warningf("Failed to save validated message: %s", err)
		}
	}
}

type MentionSlice []*Mention

func (p MentionSlice) Len() int           { return len(p) }
func (p MentionSlice) Less(i, j int) bool { return p[i].TS.Before(p[j].TS) }
func (p MentionSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

func (m *Mentions) get(ctx context.Context, target string, all bool) []*Mention {
	ret := []*Mention{}
	q := m.DS.NewQuery(MENTIONS).
		Filter("Target =", target)
	if !all {
		q = q.Filter("State =", GOOD_STATE)
	}

	it := m.DS.Client.Run(ctx, q)
	for {
		mention := &Mention{}
		_, err := it.Next(mention)
		if err == iterator.Done {
			break
		}
		if err != nil {
			m.log.Infof("Failed while reading: %s", err)
			break
		}
		ret = append(ret, mention)
	}
	sort.Sort(MentionSlice(ret))
	return ret
}

func (m *Mentions) GetAll(ctx context.Context, target string) []*Mention {
	return m.get(ctx, target, true)
}

func (m *Mentions) GetGood(ctx context.Context, target string) []*Mention {
	return m.get(ctx, target, false)
}

func (m *Mentions) UpdateState(ctx context.Context, encodedKey, state string) error {
	tx, err := m.DS.Client.NewTransaction(ctx)
	if err != nil {
		return fmt.Errorf("client.NewTransaction: %v", err)
	}
	key, err := datastore.DecodeKey(encodedKey)
	if err != nil {
		return fmt.Errorf("Unable to decode key: %s", err)
	}
	var mention Mention
	if err := tx.Get(key, &mention); err != nil {
		tx.Rollback()
		return fmt.Errorf("tx.GetMulti: %v", err)
	}
	mention.State = state
	if _, err := tx.Put(key, &mention); err != nil {
		tx.Rollback()
		return fmt.Errorf("tx.Put: %v", err)
	}
	if _, err = tx.Commit(); err != nil {
		return fmt.Errorf("tx.Commit: %v", err)
	}
	return nil
}

type MentionWithKey struct {
	Mention
	Key string
}

func (m *Mentions) GetTriage(ctx context.Context, limit, offset int) []*MentionWithKey {
	ret := []*MentionWithKey{}
	q := m.DS.NewQuery(MENTIONS).Order("-TS").Limit(limit).Offset(offset)

	it := m.DS.Client.Run(ctx, q)
	for {
		var mention Mention
		key, err := it.Next(&mention)
		if err == iterator.Done {
			break
		}
		if err != nil {
			m.log.Infof("Failed while reading: %s", err)
			break
		}
		ret = append(ret, &MentionWithKey{
			Mention: mention,
			Key:     key.Encode(),
		})
	}
	return ret
}

func (m *Mentions) GetQueued(ctx context.Context) []*Mention {
	ret := []*Mention{}
	q := m.DS.NewQuery(MENTIONS).
		Filter("State =", UNTRIAGED_STATE)

	it := m.DS.Client.Run(ctx, q)
	for {
		mention := &Mention{}
		_, err := it.Next(mention)
		if err == iterator.Done {
			break
		}
		if err != nil {
			m.log.Infof("Failed while reading: %s", err)
			break
		}
		ret = append(ret, mention)
	}
	return ret
}

func (m *Mentions) Put(ctx context.Context, mention *Mention) error {
	// TODO See if there's an existing mention already, so we don't overwrite its status?
	key := m.DS.NewKey(MENTIONS)
	key.Name = mention.key()
	if _, err := m.DS.Client.Put(ctx, key, mention); err != nil {
		return fmt.Errorf("Failed writing %#v: %s", *mention, err)
	}
	return nil
}

type UrlToImageReader func(url string) (io.ReadCloser, error)

func in(s string, arr []string) bool {
	for _, a := range arr {
		if a == s {
			return true
		}
	}
	return false
}

func firstPropAsString(uf *microformats.Microformat, key string) string {
	for _, sint := range uf.Properties[key] {
		if s, ok := sint.(string); ok {
			return s
		}
	}
	return ""
}

func (m *Mentions) findHEntry(ctx context.Context, u2r UrlToImageReader, mention *Mention, data *microformats.Data, items []*microformats.Microformat) {
	for _, it := range items {
		if in("h-entry", it.Type) {
			mention.Title = firstPropAsString(it, "name")
			if mention.Title == "" {
				mention.Title = firstPropAsString(it, "uid")
			}
			if strings.HasPrefix(mention.Title, "tag:twitter") {
				mention.Title = "Twitter"
			}
			if firstPropAsString(it, "like-of") != "" {
				mention.Title += " Like"
			}
			if firstPropAsString(it, "repost-of") != "" {
				mention.Title += " Repost"
			}
			if t, err := time.Parse(time.RFC3339, firstPropAsString(it, "published")); err == nil {
				mention.Published = t
			}
			if authorsInt, ok := it.Properties["author"]; ok {
				for _, authorInt := range authorsInt {
					if author, ok := authorInt.(*microformats.Microformat); ok {
						m.findAuthor(ctx, u2r, mention, data, author)
					}
				}
			}
		}
		m.findHEntry(ctx, u2r, mention, data, it.Children)
	}
}

type Thumbnail struct {
	PNG []byte `datastore:",noindex"`
}

func MakeUrlToImageReader(c *http.Client) UrlToImageReader {
	return func(u string) (io.ReadCloser, error) {
		resp, err := c.Get(u)
		if err != nil {
			return nil, fmt.Errorf("Error retrieving thumbnail: %s", err)
		}
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("Not a 200 response: %d", resp.StatusCode)
		}
		return resp.Body, nil
	}
}

func (m *Mentions) findAuthor(ctx context.Context, u2r UrlToImageReader, mention *Mention, data *microformats.Data, it *microformats.Microformat) {
	mention.Author = it.Value
	if len(data.Rels["author"]) > 0 {
		mention.AuthorURL = data.Rels["author"][0]
	} else {
		mention.AuthorURL = firstPropAsString(it, "url")
	}
	u := firstPropAsString(it, "photo")
	if u == "" {
		m.log.Infof("No photo URL found.")
		return
	}

	r, err := u2r(u)
	if err != nil {
		m.log.Infof("Failed to retrieve photo.")
		return
	}

	defer m.close(r)
	img, _, err := image.Decode(r)
	if err != nil {
		m.log.Infof("Failed to decode photo.")
		return
	}
	rect := img.Bounds()
	var x uint = 32
	var y uint = 32
	if rect.Max.X > rect.Max.Y {
		y = 0
	} else {
		x = 0
	}
	resized := resize.Resize(x, y, img, resize.Lanczos3)

	var buf bytes.Buffer
	encoder := png.Encoder{
		CompressionLevel: png.BestCompression,
	}
	if err := encoder.Encode(&buf, resized); err != nil {
		m.log.Errorf("Failed to encode photo.")
		return
	}

	hash := fmt.Sprintf("%x", md5.Sum(buf.Bytes()))
	t := &Thumbnail{
		PNG: buf.Bytes(),
	}
	key := m.DS.NewKey(THUMBNAIL)
	key.Name = hash
	if _, err := m.DS.Client.Put(ctx, key, t); err != nil {
		m.log.Errorf("Failed to write: %s", err)
		return
	}
	mention.Thumbnail = hash
}

func (m *Mentions) GetThumbnail(ctx context.Context, id string) ([]byte, error) {
	key := m.DS.NewKey(THUMBNAIL)
	key.Name = id
	var t Thumbnail
	if err := m.DS.Client.Get(ctx, key, &t); err != nil {
		return nil, fmt.Errorf("Failed to find image: %s", err)
	}
	return t.PNG, nil

}
