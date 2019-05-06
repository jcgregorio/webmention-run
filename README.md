webmention-run
==============

An implementation of [Webmention](https://www.w3.org/TR/webmention/) that runs
on [Google Cloud Run](https://cloud.google.com/run/docs/).

The webmentions are stored in Google Datastore, including thumbnails of authors
images if they are provided. The application has a web UI for triaging
webmentions and can automatically validate incoming webmentions in an
asynchronous manner as suggested by the W3C specification.

Prerequisites
-------------

You will need the following tools to be installed locally:

  - Docker
  - Google Cloud command line tools
  - Go
  - A unixy type of environment that includes GNU Make.

Installation
------------

[First set up Google Cloud Run](https://cloud.google.com/run/docs/setup)

Then clone this repository and edit config.mk:

    export PROJECT=my-google-project-name
    export REGION=us-central1
    export HOST=https://webmention-blahblah.a.run.app
    export CLIENT_ID=95264313...ps.googleusercontent.com
    export DATASTORE_NAMESPACE=blog
    export ADMINS=someone@example.com
    export PORT=1313

**PROJECT** - The name of your Google Cloud Project.

**REGION** - The serving region, e.g. us-central1.

**HOST** - The scheme and domain name where this is running. If you are using
  the randomly generated domain name that Google Cloud Run supplies you will
  have to deploy this application first, find the name, update config.mk, and
  then redeploy with the correct HOST value.

**CLIENT_ID** - Google Sign-In for Websites is used to protect the `/`
  endpoint used to manually triage webmentions. See the [Google Sign-In for
  Websites](https://developers.google.com/identity/sign-in/web/sign-in) page
  for how to configure a client id. Note that you need to know the domain
  name you are serving off of, so just like HOST, you may need to deploy the
  application first, find the domain name, and then update the configuration
  for the client.

**DATASTORE_NAMESPACE** - The namespace in the Google Cloud Datastore under
  which webmention data will be stored. Note that no indices are needed.

**ADMINS** - A comma separated list of users who are allowed to manually
  triage webmentions.

**PORT** - Used only for local testing, this is the port that the application
  should listen on for HTTP requests.

To build and push a docker image to your Google Cloud Container Registry:

    make release

If that was successful then deploy the image to Google Run:

    make push

Using
-----

Now that the application is running you can add the following to your sites
`head`:

    <link href="$HOST/IncomingWebMention" rel="webmention" />

where $HOST is the value of HOST you set in `config.mk`. This indicates
that you site is capable of receiving webmentions. Put that link tag
on every page you want to receive webmentions.

You can visit

    $HOST/

to manually triage incoming webmentions. If you want to automatically triage
webmentions by confirming that the source link really does contain a link
to your page then you can set up a cron job to visit:

    $HOST/VerifyQueuedMentions

You can use [Google Cloud Scheduler](https://cloud.google.com/scheduler/) for this. I use this crontab
rule to validate webmentions every 5 minutes:

    */5 * * * *

Now the only thing left is to display the webmentions on the pages that have
received them. The application returns HTML describing the webmentions
from the `/Mentions` endpoint. You can run JS on each page to dynamically
include the approved webmentions like so:

```
<div id=mentions></div>

...

<script type="text/javascript" charset="utf-8">
  fetch('https://HOST/Mentions', {
    cache: 'no-cache',
  }).then(function(resp) {
    if (!resp.ok) {
      return
    }
    resp.text().then(function(text) {
      document.getElementById('mentions').innerHTML = text;
    });
  });
</script>
```

Where `HOST` should be replaced with the domain name where the application is
running.

Test
----

You will need to have the Google Cloud Datastore Emulator running locally
to run all the unit tests. First run:

    make start_datastore_emulator

Then in a separate shell run the following from within this directory:

    make test
