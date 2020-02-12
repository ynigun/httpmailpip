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
	fcgiclient "github.com/tomasen/fcgi_client"
)

type fcgiConfig struct {
	// full path to script for the save mail task
	// eg. /home/user/scripts/save.php
	//ScriptFileNameNameSave string `json:"fcgi_script_filename_save"`
	HttpPipSave string `json:"HttpPipSave"`
	// full path to script for recipient validation
	// eg /home/user/scripts/val_rcpt.php
	HttpPipValidate string `json:"HttpPipValidate"`
	//ScriptFileNameNameValidate string `json:"fcgi_script_filename_validate"`
	// "tcp" or "unix"
	ConnectionType string `json:"fcgi_connection_type"`
	// where to Dial, eg "/tmp/php-fpm.sock" for unix-socket or "127.0.0.1:9000" for tcp
	ConnectionAddress string `json:"fcgi_connection_address"`
}

type FastCGIProcessor struct {
	config *fcgiConfig
	client *fcgiclient.FCGIClient
}

func newFastCGIProcessor(config *fcgiConfig) (*FastCGIProcessor, error) {
	p := &FastCGIProcessor{}
	p.config = config
	err := p.connect()
	if err != nil {
		backends.Log().Debug("FastCgi error", err)
		return p, err
	}
	return p, err
}

func (f *FastCGIProcessor) connect() (err error) {
	backends.Log().Debug("connecting to fcgi:", f.config.ConnectionType, f.config.ConnectionAddress)
	f.client, err = fcgiclient.Dial(f.config.ConnectionType, f.config.ConnectionAddress)
	return err
}

// get sends a get query to script with q query values
func (f *FastCGIProcessor) get(script string, q url.Values) (result []byte, err error) {
	//result, err = exec.Command("gorun", "/root/pipmail.go").Output()
	//if err != nil {
	//	log.Fatal(err)
	//}
	url := fmt.Sprintf("%s?validate=1", f.config.HttpPipValidate)
	//url := fmt.Sprintf("http://51.15.47.193:8090/test")
	resp, _ := http.Get(url)
	//		resp, _ := http.Get("http://51.15.47.193:8090/ppp/")
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		print(err)
	}
	fmt.Print(string(body))
	result = body
	return result, nil

}

func (f *FastCGIProcessor) postSave(e *mail.Envelope) (result []byte, err error) {
	//cmd := result
	/*env["remote_ip"] = e.RemoteIP
	env["subject"] = e.Subject
	env["helo"] = e.Helo
	env["mail_from"] = e.MailFrom.String()
	env["body"] = e.String()
	*/
	//datatest := fmt.Sprintf("body=%q", e.Data.String())
	/*if err := e.ParseHeaders(); err != nil && err != io.EOF {
		backends.Log().Debug("err", err)
	}*/
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
	//data.Set("surname", "bar")

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
	urlStr := u.String() // "https://api.com/user/"

	client := &http.Client{}
	r, _ := http.NewRequest("POST", urlStr, strings.NewReader(data.Encode())) // URL-encoded payload
	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Add("Content-Length", strconv.Itoa(len(data.Encode())))

	resp, _ := client.Do(r)
	fmt.Println(resp.Status)
	//	url := fmt.Sprintf("%s?body=%q", f.config.HttpPipSave, e.String())
	//	backends.Log().Debug("la", url)
	//	//backends.Log().Debug("data", e.Data.String())
	//	//url := fmt.Sprintf("http://51.15.47.193:8090/test")
	//	resp, _ := http.Get(url)
	//	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	//
	//	fmt.Print(string(body))
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
		backends.Log().Info("testing script:", p.config.HttpPipValidate)
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

					backends.Log().Debug("FastCgi  Read Body ", string(result))

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
