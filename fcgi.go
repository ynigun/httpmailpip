package fcgi_processor

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/DusanKasan/parsemail"
	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/mail"
	"github.com/flashmob/go-guerrilla/response"
)

type fcgiConfig struct {
	
	HttpPipSave string `json:"HttpPipSave"`
	
	HttpPipValidate string `json:"HttpPipValidate"`
	
}

type FastCGIProcessor struct {
	config *fcgiConfig
}

func newFastCGIProcessor(config *fcgiConfig) (*FastCGIProcessor, error) {
	p := &FastCGIProcessor{}
	p.config = config
	
	return p, nil
}



// get sends a get query to script with q query values
func (f *FastCGIProcessor) get(script string, q url.Values) (result []byte, err error) {
	
	url := fmt.Sprintf("%s?validate=%s", f.config.HttpPipValidate,q.Encode())
	resp, _ := http.Get(url)
		defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		print(err)
	}
	result = body
	return result, nil

}

func (f *FastCGIProcessor) postSave(e *mail.Envelope) (result []byte, err error) {
	
	apiUrl := f.config.HttpPipSave
	resource := "/user/"
	var reader io.Reader
	// this reads an email message
	reader = &e.Data
	email, err := parsemail.Parse(reader) // returns Email struct and error
	if err != nil {
		// handle error
	}
	/*
	   fmt.Println(email.Subject)
	   fmt.Println(email.From)
	   fmt.Println(email.To)
	   fmt.Println(email.HTMLBody)
	*/data := url.Values{}

	for i := range e.RcptTo {
		data.Set(fmt.Sprintf("rcpt_to_%d", i), e.RcptTo[i].String())
	}
	data.Set("remote_ip", e.RemoteIP)
	data.Set("subject", e.Subject)
	data.Set("tls_on", strconv.FormatBool(e.TLS))
	data.Set("helo", e.Helo)
	data.Set("mail_from", e.MailFrom.String())
	data.Set("body", email.HTMLBody)
	u, _ := url.ParseRequestURI(apiUrl)
	u.Path = resource
	urlStr := u.String() 

	client := &http.Client{}
	r, _ := http.NewRequest("POST", urlStr, strings.NewReader(data.Encode())) // URL-encoded payload
	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Add("Content-Length", strconv.Itoa(len(data.Encode())))

	resp, _ := client.Do(r)
	
	body, _ := ioutil.ReadAll(resp.Body)
	
	result = body
	return result, nil
}

var Processor = func() backends.Decorator {

	// The following initialization is run when the program first starts

	// config will be populated by the initFunc
	var (
		p *FastCGIProcessor
	)
	// initFunc is an initializer function which is called when our processor gets created.
	// It gets called for every worker
	initializer := backends.InitializeWith(func(backendConfig backends.BackendConfig) error {
		configType := backends.BaseConfig(&fcgiConfig{})
		bcfg, err := backends.Svc.ExtractConfig(backendConfig, configType)

		if err != nil {
			return err
		}
		c := bcfg.(*fcgiConfig)
		p, err = newFastCGIProcessor(c)
		if err != nil {
			return err
		}
		p.config = c
		// test the settings
		v := url.Values{}
		v.Set("rcpt_to", "test@example.com")
		result, err := p.get(p.config.HttpPipValidate, v)
		if err != nil {
			backends.Log().WithError(err).Error("could get fcgi to work")
			return nil
		} else {
			backends.Log().Debug(result)
		}
		return nil
	})
	// register our initializer
	backends.Svc.AddInitializer(initializer)

	return func(c backends.Processor) backends.Processor {
		// The function will be called on each email transaction.
		// On success, it forwards to the next step in the processor call-stack,
		// or returns with an error if failed
		return backends.ProcessWith(func(e *mail.Envelope, task backends.SelectTask) (backends.Result, error) {
			if task == backends.TaskValidateRcpt {
				// Check the recipients for each RCPT command.
				// This is called each time a recipient is added,
				// validate only the _last_ recipient that was appended
				if size := len(e.RcptTo); size > 0 {
					v := url.Values{}
					v.Set("rcpt_to", e.RcptTo[len(e.RcptTo)-1].String())
					result, err := p.get(p.config.HttpPipValidate, v)
					if err != nil {
						backends.Log().Debug("FastCgi error", err)
						return backends.NewResult(
								response.Canned.FailNoSenderDataCmd),
							backends.StorageNotAvailable
					}


					if string(result[0:6]) == "PASSED" {
						// validation passed
						return c.Process(e, task)
					} else {
						// validation failed
						backends.Log().Debug("FastCgi test Read Body failed", err)
						return backends.NewResult(
								response.Canned.FailNoSenderDataCmd),
							backends.StorageNotAvailable
					}

					return c.Process(e, task)

				}
				return c.Process(e, task)
			} else if task == backends.TaskSaveMail {
				for i := range e.RcptTo {
					// POST to FCGI
					resp, err := p.postSave(e)
					if err != nil {

					} else if strings.Index(string(resp), "SAVED") == 0 {
						return c.Process(e, task)
					} else {
						return c.Process(e, task)
						backends.Log().WithError(err).Error("Could not save email")
						return backends.NewResult(fmt.Sprintf("554 Error: could not save email for [%s]", e.RcptTo[i])), err
					}
				}
				// continue to the next Processor in the decorator chain
				return c.Process(e, task)
			} else {
				return c.Process(e, task)
			}

		})
	}
}
