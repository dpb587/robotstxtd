package inspector

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"strconv"

	"github.com/dpb587/robotstxtd/robotstxtdpb"
	"github.com/dpb587/robotstxtd/server"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

//go:embed static/htmx.min.js static/app.css
var staticFS embed.FS

//go:embed templates/*.html
var templateFS embed.FS

type Handler struct {
	svc   *server.Service
	pages map[string]*template.Template
	mux   *http.ServeMux
}

type page struct {
	Title string
	Nav   string
	Body  any
}

type botView struct {
	Identifier string
	Version    string
	URL        string
	Email      string
	Client     string
}

type fileRow struct {
	ServiceURI  string
	Generation  string
	RequestTime string
	StaleTime   string
	Status      string
	StatusClass string
	Invalidated bool
}

type fileList struct {
	Rows               []fileRow
	NextPageToken      string
	IncludeInvalidated bool
	PageSize           uint32
}

type fileDetail struct {
	ServiceURI  string
	Generation  string
	Requested   string
	URI         string
	Status      string
	StatusClass string
	Headers     string
	Body        string
	Truncated   bool
	Failed      bool
}

type evalView struct {
	Error      string
	Allowed    bool
	Decision   string
	Reason     string
	ServiceURI string
	Generation string
}

func NewHandler(svc *server.Service) (http.Handler, error) {
	pages := map[string][]string{
		"bot":        {"templates/layout.html", "templates/bot.html"},
		"files":      {"templates/layout.html", "templates/files.html", "templates/file_rows.html"},
		"file":       {"templates/layout.html", "templates/file.html"},
		"evaluate":   {"templates/layout.html", "templates/evaluate.html"},
		"invalidate": {"templates/invalidate.html"},
	}

	parsed := make(map[string]*template.Template, len(pages))
	for name, files := range pages {
		t, err := template.ParseFS(templateFS, files...)
		if err != nil {
			return nil, fmt.Errorf("template %s: %v", name, err)
		}
		parsed[name] = t
	}

	static, err := fs.Sub(staticFS, "static")
	if err != nil {
		return nil, fmt.Errorf("static: %v", err)
	}

	h := &Handler{
		svc:   svc,
		pages: parsed,
	}

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(static))))
	mux.HandleFunc("GET /{$}", h.botPage)
	mux.HandleFunc("GET /files", h.filesPage)
	mux.HandleFunc("GET /file", h.filePage)
	mux.HandleFunc("POST /file/invalidate", h.invalidate)
	mux.HandleFunc("GET /evaluate", h.evaluatePage)
	mux.HandleFunc("POST /evaluate", h.evaluate)
	mux.HandleFunc("POST /api/GetBot", h.apiGetBot)
	mux.HandleFunc("POST /api/GetFile", h.apiGetFile)
	mux.HandleFunc("POST /api/ListFiles", h.apiListFiles)
	mux.HandleFunc("POST /api/InvalidateFileCache", h.apiInvalidate)
	mux.HandleFunc("POST /api/EvaluateUri", h.apiEvaluate)
	h.mux = mux

	return h, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) render(w http.ResponseWriter, name, tmpl string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.pages[name].ExecuteTemplate(w, tmpl, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) botPage(w http.ResponseWriter, r *http.Request) {
	bot, err := h.svc.GetBot(r.Context(), &robotstxtdpb.Crawler_GetBot_Request{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.render(w, "bot", "layout", &page{
		Title: "Bot · robotstxtd",
		Nav:   "bot",
		Body:  struct{ Bot botView }{Bot: botFromProto(bot)},
	})
}

func (h *Handler) filesPage(w http.ResponseWriter, r *http.Request) {
	pageSize, err := queryPageSize(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	list, err := h.listFiles(r, pageSize)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if r.URL.Query().Get("partial") == "1" {
		h.render(w, "files", "file-rows", list)
		return
	}

	h.render(w, "files", "layout", &page{
		Title: "Files · robotstxtd",
		Nav:   "files",
		Body:  list,
	})
}

func (h *Handler) listFiles(r *http.Request, pageSize uint32) (*fileList, error) {
	res, err := h.svc.ListFiles(r.Context(), &robotstxtdpb.Crawler_ListFiles_Request{
		PageSize:           pageSize,
		PageToken:          r.URL.Query().Get("page_token"),
		IncludeInvalidated: r.URL.Query().Get("include_invalidated") == "1",
	})
	if err != nil {
		return nil, err
	}

	if pageSize == 0 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}

	rows := make([]fileRow, 0, len(res.GetFiles()))
	for _, file := range res.GetFiles() {
		statusText, statusClass := summaryStatus(file)
		rows = append(rows, fileRow{
			ServiceURI:  file.GetServiceUri(),
			Generation:  file.GetGeneration(),
			RequestTime: formatTime(file.GetRequestTime()),
			StaleTime:   formatTime(file.GetStaleTime()),
			Status:      statusText,
			StatusClass: statusClass,
			Invalidated: file.GetInvalidated(),
		})
	}

	return &fileList{
		Rows:               rows,
		NextPageToken:      res.GetNextPageToken(),
		IncludeInvalidated: r.URL.Query().Get("include_invalidated") == "1",
		PageSize:           pageSize,
	}, nil
}

func (h *Handler) filePage(w http.ResponseWriter, r *http.Request) {
	serviceURI := r.URL.Query().Get("service_uri")
	if serviceURI == "" {
		http.Error(w, "missing service_uri", http.StatusBadRequest)
		return
	}

	file, err := h.svc.GetFile(r.Context(), &robotstxtdpb.Crawler_GetFile_Request{
		ServiceUri: serviceURI,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	h.render(w, "file", "layout", &page{
		Title: serviceURI + " · robotstxtd",
		Nav:   "files",
		Body:  struct{ Detail fileDetail }{Detail: detailFromProto(file)},
	})
}

func (h *Handler) invalidate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	serviceURI := r.PostFormValue("service_uri")
	if serviceURI == "" {
		http.Error(w, "missing service_uri", http.StatusBadRequest)
		return
	}

	_, err := h.svc.InvalidateFileCache(r.Context(), &robotstxtdpb.Crawler_InvalidateFileCache_Request{
		ServiceUri: serviceURI,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	h.render(w, "invalidate", "invalidate-result", struct{ ServiceURI string }{ServiceURI: serviceURI})
}

func (h *Handler) evaluatePage(w http.ResponseWriter, r *http.Request) {
	bot, err := h.svc.GetBot(r.Context(), &robotstxtdpb.Crawler_GetBot_Request{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.render(w, "evaluate", "layout", &page{
		Title: "Evaluate · robotstxtd",
		Nav:   "evaluate",
		Body: struct {
			BotIdentifier string
			URI           string
			Result        *evalView
		}{
			BotIdentifier: bot.GetIdentifier(),
		},
	})
}

func (h *Handler) evaluate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	uri := r.PostFormValue("uri")
	result := &evalView{}
	res, err := h.svc.EvaluateUri(r.Context(), &robotstxtdpb.Crawler_EvaluateUri_Request{Uri: uri})
	if err != nil {
		result.Error = err.Error()
	} else {
		result = evalFromProto(res)
	}

	if r.Header.Get("HX-Request") != "" {
		h.render(w, "evaluate", "evaluate-result", result)
		return
	}

	bot, err := h.svc.GetBot(r.Context(), &robotstxtdpb.Crawler_GetBot_Request{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.render(w, "evaluate", "layout", &page{
		Title: "Evaluate · robotstxtd",
		Nav:   "evaluate",
		Body: struct {
			BotIdentifier string
			URI           string
			Result        *evalView
		}{
			BotIdentifier: bot.GetIdentifier(),
			URI:           uri,
			Result:        result,
		},
	})
}

func (h *Handler) apiGetBot(w http.ResponseWriter, r *http.Request) {
	req := &robotstxtdpb.Crawler_GetBot_Request{}
	if !readProto(w, r, req) {
		return
	}
	res, err := h.svc.GetBot(r.Context(), req)
	writeRPC(w, res, err)
}

func (h *Handler) apiGetFile(w http.ResponseWriter, r *http.Request) {
	req := &robotstxtdpb.Crawler_GetFile_Request{}
	if !readProto(w, r, req) {
		return
	}
	res, err := h.svc.GetFile(r.Context(), req)
	writeRPC(w, res, err)
}

func (h *Handler) apiListFiles(w http.ResponseWriter, r *http.Request) {
	req := &robotstxtdpb.Crawler_ListFiles_Request{}
	if !readProto(w, r, req) {
		return
	}
	res, err := h.svc.ListFiles(r.Context(), req)
	writeRPC(w, res, err)
}

func (h *Handler) apiInvalidate(w http.ResponseWriter, r *http.Request) {
	req := &robotstxtdpb.Crawler_InvalidateFileCache_Request{}
	if !readProto(w, r, req) {
		return
	}
	res, err := h.svc.InvalidateFileCache(r.Context(), req)
	writeRPC(w, res, err)
}

func (h *Handler) apiEvaluate(w http.ResponseWriter, r *http.Request) {
	req := &robotstxtdpb.Crawler_EvaluateUri_Request{}
	if !readProto(w, r, req) {
		return
	}
	res, err := h.svc.EvaluateUri(r.Context(), req)
	writeRPC(w, res, err)
}

func readProto(w http.ResponseWriter, r *http.Request, msg proto.Message) bool {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeStatus(w, status.New(codes.InvalidArgument, "invalid json"))
		return false
	}
	if len(body) == 0 {
		return true
	}
	if err := protojson.Unmarshal(body, msg); err != nil {
		writeStatus(w, status.New(codes.InvalidArgument, "invalid json"))
		return false
	}
	return true
}

func writeRPC(w http.ResponseWriter, msg proto.Message, err error) {
	if err != nil {
		writeStatus(w, status.Convert(err))
		return
	}
	writeProto(w, http.StatusOK, msg)
}

func writeStatus(w http.ResponseWriter, st *status.Status) {
	writeProto(w, httpStatus(st.Code()), st.Proto())
}

func writeProto(w http.ResponseWriter, code int, msg proto.Message) {
	buf, err := protojson.Marshal(msg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(buf)
}

func httpStatus(code codes.Code) int {
	switch code {
	case codes.OK:
		return http.StatusOK
	case codes.InvalidArgument:
		return http.StatusBadRequest
	case codes.NotFound:
		return http.StatusNotFound
	case codes.Unavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

func queryPageSize(r *http.Request) (uint32, error) {
	raw := r.URL.Query().Get("page_size")
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid page_size")
	}
	return uint32(n), nil
}

func botFromProto(bot *robotstxtdpb.UserAgent) botView {
	return botView{
		Identifier: bot.GetIdentifier(),
		Version:    bot.GetVersion(),
		URL:        bot.GetUrl(),
		Email:      bot.GetEmail(),
		Client:     bot.ClientString(),
	}
}

func summaryStatus(file *robotstxtdpb.Crawler_ListFiles_Summary) (string, string) {
	if errStatus := file.GetError(); errStatus != nil {
		return errStatus.GetMessage(), "text-red-700"
	}
	if res := file.GetResponse(); res != nil {
		code := res.GetStatusCode()
		if code >= 200 && code < 300 {
			return fmt.Sprintf("%d", code), "text-emerald-800"
		}
		return fmt.Sprintf("%d", code), "text-amber-800"
	}
	return "", "text-stone-500"
}

func detailFromProto(file *robotstxtdpb.File) fileDetail {
	detail := fileDetail{
		ServiceURI: file.GetAuthority().GetServiceUri(),
		Generation: file.GetAuthority().GetGeneration(),
		Requested:  formatTime(file.GetUpstream().GetTime()),
		URI:        file.GetUpstream().GetUri(),
	}
	switch result := file.GetUpstream().GetResult().(type) {
	case *robotstxtdpb.File_Upstream_Error:
		detail.Failed = true
		detail.Status = result.Error.GetMessage()
		detail.StatusClass = "text-red-700"
	case *robotstxtdpb.File_Upstream_Response_:
		detail.Status = fmt.Sprintf("%d", result.Response.GetStatusCode())
		detail.Headers = result.Response.GetHeaders()
		if result.Response.GetStatusCode() >= 200 && result.Response.GetStatusCode() < 300 {
			detail.StatusClass = "text-emerald-800"
		} else {
			detail.StatusClass = "text-amber-800"
		}
		if body := file.GetBody(); body != nil {
			detail.Body = body.GetText()
			detail.Truncated = body.GetTruncated()
		}
	}
	return detail
}

func evalFromProto(res *robotstxtdpb.UriEvaluation) *evalView {
	view := &evalView{
		ServiceURI: res.GetAuthority().GetServiceUri(),
		Generation: res.GetAuthority().GetGeneration(),
	}
	switch access := res.Access.(type) {
	case *robotstxtdpb.UriEvaluation_AllowAccess:
		view.Allowed = true
		view.Decision = "Allowed"
		switch reason := access.AllowAccess.GetReason().(type) {
		case *robotstxtdpb.UriEvaluation_Access_Allow_UnavailableStatus:
			view.Reason = "Unavailable status"
		case *robotstxtdpb.UriEvaluation_Access_Allow_NoRuleMatch:
			view.Reason = "No matching rule"
		case *robotstxtdpb.UriEvaluation_Access_Allow_RuleMatchLine:
			view.Reason = fmt.Sprintf("Allow rule on line %d", reason.RuleMatchLine)
		}
	case *robotstxtdpb.UriEvaluation_DisallowAccess:
		view.Decision = "Disallowed"
		switch reason := access.DisallowAccess.GetReason().(type) {
		case *robotstxtdpb.UriEvaluation_Access_Disallow_UnreachableStatus:
			view.Reason = "Unreachable status"
		case *robotstxtdpb.UriEvaluation_Access_Disallow_RuleMatchLine:
			view.Reason = fmt.Sprintf("Disallow rule on line %d", reason.RuleMatchLine)
		}
	}
	return view
}

func formatTime(ts *timestamppb.Timestamp) string {
	if ts == nil {
		return ""
	}
	return ts.AsTime().UTC().Format("2006-01-02 15:04:05 UTC")
}
