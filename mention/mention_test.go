package mention

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	_ "image/gif"
	_ "image/jpeg"

	"github.com/jcgregorio/logger"
	"github.com/stretchr/testify/assert"
	"willnorris.com/go/microformats"
)

// InitDatastore is a common utility function used in tests. It sets up the
// datastore to connect to the emulator and also clears out all instances of
// the given 'kinds' from the datastore.
func InitForTesting(t assert.TestingT) *Mentions {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	emulatorHost := os.Getenv("DATASTORE_EMULATOR_HOST")
	if emulatorHost == "" {
		assert.Fail(t, `Running tests that require a running Cloud Datastore emulator.

Run

	"gcloud beta emulators datastore start --no-store-on-disk --host-port=localhost:8888"

and then run

  $(gcloud beta emulators datastore env-init)

to set the environment variables. When done running tests you can unset the env variables:

  $(gcloud beta emulators datastore env-unset)

`)
	}

	// Do a quick healthcheck against the host, which will fail immediately if it's down.
	_, err := http.DefaultClient.Get("http://" + emulatorHost + "/")
	assert.NoError(t, err, fmt.Sprintf("Cloud emulator host %s appears to be down or not accessible.", emulatorHost))

	m, err := NewMentions(context.Background(), "test-project", fmt.Sprintf("test-namespace-%d", r.Uint64()), logger.New())
	assert.NoError(t, err)
	return m
}

func TestDB(t *testing.T) {
	m := InitForTesting(t)

	err := m.Put(context.Background(), &Mention{
		Source: "https://stackoverflow.com/foo",
		Target: "https://bitworking.org/bar",
		State:  GOOD_STATE,
		TS:     time.Now(),
	})
	assert.NoError(t, err)

	err = m.Put(context.Background(), &Mention{
		Source: "https://spam.com/foo",
		Target: "https://bitworking.org/bar",
		State:  SPAM_STATE,
		TS:     time.Now(),
	})
	assert.NoError(t, err)

	err = m.Put(context.Background(), &Mention{
		Source: "https://news.ycombinator.com/foo",
		Target: "https://bitworking.org/bar",
		State:  GOOD_STATE,
		TS:     time.Now(),
	})
	assert.NoError(t, err)
	time.Sleep(2)

	mentions := m.GetGood(context.Background(), "https://bitworking.org/bar")
	assert.Len(t, mentions, 2)
}

func TestParseMicroformats(t *testing.T) {
	raw := `<article class="post h-entry" itemscope="" itemtype="http://schema.org/BlogPosting">

	<header class="post-header">
	<h1 class="post-title p-name" itemprop="name headline">WebMention Only</h1>
	<p class="post-meta">
	<a class="u-url" href="/news/2018/01/webmention-only">
    	<time datetime="2018-01-13T00:00:00-05:00" itemprop="datePublished" class="dt-published"> Jan 13, 2018 </time>
	</a>
	•
	<a rel="author" class="p-author h-card" href="/about">
	    <span itemprop="author" itemscope="" itemtype="http://schema.org/Person">
	        <img class="u-photo" src="/images/joe2016.jpg" alt="" style="height: 16px; border-radius: 8px; margin-right: 4px;">
	        <span itemprop="name">Joe Gregorio</span>
			</span>
	  </a>
	</p>
	</header>

	<div class="post-content e-content" itemprop="articleBody">
	<p><a href="https://allinthehead.com/retro/378/implementing-webmentions">Drew McLellan has gone WebMention-only.</a></p>

	<p>It’s an interesting idea, though I will still probably build a comment system
	for this blog and replace Disqus.</p>

	</div>
	<div id="mentions"></div>
</article>`

	m := InitForTesting(t)

	reader := bytes.NewReader([]byte(raw))
	u, err := url.Parse("https://bitworking.org/news/2018/01/webmention-only")
	assert.NoError(t, err)
	data := microformats.Parse(reader, u)
	mention := &Mention{
		Source: "https://bitworking.org/news/2018/01/webmention-only",
	}
	urlToImageReader := func(url string) (io.ReadCloser, error) {
		return os.Open("./testdata/author_image.jpg")
	}
	m.findHEntry(context.Background(), urlToImageReader, mention, data, data.Items)
	assert.Equal(t, "Joe Gregorio", mention.Author)
	assert.Equal(t, "2018-01-13 00:00:00 -0500 EST", mention.Published.String())
	assert.Equal(t, "f3f799d1a61805b5ee2ccb5cf0aebafa", mention.Thumbnail)
	assert.Equal(t, "https://bitworking.org/about", mention.AuthorURL)
}
