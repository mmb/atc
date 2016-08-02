package login

import (
	"html/template"
	"net/http"

	"github.com/concourse/atc/web"
	"github.com/pivotal-golang/lager"
	"github.com/tedsuo/rata"
)

type handler struct {
	logger        lager.Logger
	clientFactory web.ClientFactory
	template      *template.Template
}

func NewHandler(
	logger lager.Logger,
	clientFactory web.ClientFactory,
	template *template.Template,
) http.Handler {
	return &handler{
		logger:        logger,
		clientFactory: clientFactory,
		template:      template,
	}
}

type TemplateData struct {
	TeamName string
	Redirect string
}

func (handler *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	redirect := r.FormValue("redirect")
	if redirect == "" {
		indexPath, err := web.Routes.CreatePathForRoute(web.Index, rata.Params{})
		if err != nil {
			handler.logger.Error("failed-to-generate-index-path", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		redirect = indexPath
	}

	teamName := r.FormValue(":team_name")

	err := handler.template.Execute(w, TemplateData{
		TeamName: teamName,
		Redirect: redirect,
	})
	if err != nil {
		handler.logger.Info("failed-to-generate-login-template", lager.Data{
			"error": err.Error(),
		})
	}
}
