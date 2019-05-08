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
    	<time datetime="2018-01-13T00:00:00-05:00" itemprop="datePublished" class="dt-published"> Jan 13, 2018 </time>
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
	u, err := url.Parse("https://bitworking.org/news/2018/01/webmention-only-2")
	assert.NoError(t, err)
	data := microformats.Parse(reader, u)
	// VerifyQueuedMentions will start with Source == URL.
	mention := &Mention{
		Source: "https://bitworking.org/news/2018/01/webmention-only-2",
		URL:    "https://bitworking.org/news/2018/01/webmention-only-2",
	}
	urlToImageReader := func(url string) (io.ReadCloser, error) {
		return os.Open("./testdata/author_image.jpg")
	}
	m.findHEntry(context.Background(), urlToImageReader, mention, data, data.Items)
	assert.Equal(t, "Joe Gregorio", mention.Author)
	assert.Equal(t, "2018-01-13 00:00:00 -0500 EST", mention.Published.String())
	assert.Equal(t, "f3f799d1a61805b5ee2ccb5cf0aebafa", mention.Thumbnail)
	assert.Equal(t, "https://bitworking.org/about", mention.AuthorURL)
	assert.Equal(t, "https://bitworking.org/news/2018/01/webmention-only-2", mention.URL)
}

func TestParseMicroformatsBridgy(t *testing.T) {
	raw := `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta http-equiv="refresh" content="0;url=https://twitter.com/bitworking/status/1125545560939933697#favorited-by-8855932">
<title>Some Body</title>
<style type="text/css">
body {
  font-family: 'Helvetica Neue', Helvetica, Arial, sans-serif;
}
.p-uid {
  display: none;
}
.u-photo {
  max-width: 50px;
  border-radius: 4px;
}
.e-content {
  margin-top: 10px;
  font-size: 1.3em;
}
</style>
</head>
<article class="h-entry">
  <span class="p-uid">tag:twitter.com,2013:1125545560939933697_favorited_by_8855932</span>
  
  
  
  <span class="p-author h-card">
    <data class="p-uid" value="tag:twitter.com,2013:somebody"></data>
<data class="p-numeric-id" value="8855932"></data>
    <a class="p-name u-url" href="https://twitter.com/somebody">Some Body</a>
    <span class="p-nickname">somebody</span>
    <img class="u-photo" src="https://pbs.twimg.com/profile_images/81145045/SmmSmallPortrait.JPG" alt="" />
  </span>

  <a class="p-name u-url" href="https://twitter.com/bitworking/status/1125545560939933697#favorited-by-8855932"></a>
  <div class="">
  
  
  </div>
<a class="u-like-of" href="https://twitter.com/bitworking/status/1125545560939933697"></a>
<a class="u-like-of" href="https://bitworking.org/news/2019/05/webmention-on-google-cloud-run"></a>
</article>
</html> `

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
	assert.Equal(t, "Some Body", mention.Author)
	assert.Equal(t, "Twitter Like", mention.Title)
	assert.Equal(t, "f3f799d1a61805b5ee2ccb5cf0aebafa", mention.Thumbnail)
	assert.Equal(t, "https://twitter.com/somebody", mention.AuthorURL)
	assert.Equal(t, "https://twitter.com/bitworking/status/1125545560939933697#favorited-by-8855932", mention.URL)
}
