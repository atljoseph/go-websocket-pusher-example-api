package handlers

import (
	"net/http"

	"github.com/sirupsen/logrus"
)

func HomeHttpHandler(w http.ResponseWriter, r *http.Request) {
	logrus.WithFields(logrus.Fields{
		"r.URL": r.URL,
	}).Infof("HomeHttpHandler")
	if r.URL.Path != "/" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	logrus.WithFields(logrus.Fields{
		"r.URL": r.URL,
	}).Infof("HomeHttpHandler:ServeFile")
	http.ServeFile(w, r, "home.html")
}
