package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/go-macaron/binding"
	"github.com/go-macaron/cache"
	"github.com/go-macaron/session"
	"github.com/martini-contrib/cors"
	"github.com/md2k/gofe/fe"
	"github.com/md2k/gofe/models"
	"github.com/md2k/gofe/settings"
	macaron "gopkg.in/macaron.v1"
)

var DEFAULT_API_ERROR_RESPONSE = models.GenericResp{
	models.GenericRespBody{false, "Not Supported by current backend."},
}

type SessionInfo struct {
	User         string
	Password     string
	FileExplorer fe.FileExplorer
	Uid          string
}

func main() {
	configRuntime()
	startServer()
}

func configRuntime() {
	if os.Getenv("GOGC") == "" {
		log.Printf("Setting default GOGC=%d", 800)
		debug.SetGCPercent(800)
	} else {
		log.Printf("Using GOGC=%s from env.", os.Getenv("GOGC"))
	}

	if os.Getenv("GOMAXPROCS") == "" {
		numCPU := runtime.NumCPU()
		log.Printf("Setting default GOMAXPROCS=%d.", numCPU)
		runtime.GOMAXPROCS(numCPU)
	} else {
		log.Printf("Using GOMAXPROCS=%s from env", os.Getenv("GOMAXPROCS"))
	}
}

func startServer() {
	settings.Load()
	macaron.Classic()
	m := macaron.New()
	m.Use(macaron.Logger())
	m.Use(macaron.Recovery())
	if len(settings.Server.Statics) > 0 {
		m.Use(macaron.Statics(macaron.StaticOptions{
			Prefix:      "static",
			SkipLogging: false,
		}, settings.Server.Statics...))
	}
	m.Use(cache.Cacher())
	m.Use(cors.Allow(&cors.Options{
		AllowOrigins:     settings.Server.CorsOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
		AllowHeaders:     []string{"Origin", "X-Requested-With", "Content-Type"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))
	m.Use(session.Sessioner())
	m.Use(macaron.Renderer())
	m.Use(Contexter())

	m.Post("/api/_", binding.Bind(models.GenericReq{}), apiHandler)
	m.Post("/bridges/php/handler.php", binding.Bind(models.GenericReq{}), apiHandler)
	m.Get("/", mainHandler)
	m.Get("/login", loginHandler)
	m.Get("/api/download", downloadHandler)
	m.Post("/api/upload", uploadHandler)

	if settings.Server.Type == "http" {
		bind := strings.Split(settings.Server.Bind, ":")
		if len(bind) == 1 {
			m.Run(bind[0])
		}
		if len(bind) == 2 {
			m.Run(bind[0], bind[1])
		}
	} else if settings.Server.Type == "https" {
		log.Fatal(http.ListenAndServeTLS(settings.Server.Bind, settings.Server.SSLCert, settings.Server.SSLKey, m))
	}
}

func mainHandler(ctx *macaron.Context) {
	ctx.HTML(200, "index")
}

func loginHandler(ctx *macaron.Context) {
	ctx.HTML(200, "login")
}

func defaultHandler(ctx *macaron.Context) {
	ctx.JSON(200, DEFAULT_API_ERROR_RESPONSE)
}

func apiHandler(c *macaron.Context, req models.GenericReq, s SessionInfo) {
	switch req.Action {
	case "list":
		ls, err := s.FileExplorer.ListDir(req.Path)
		if err == nil {
			c.JSON(200, models.ListDirResp{ls})
		} else {
			ApiErrorResponse(c, 400, err)
		}
	case "rename":
		err := s.FileExplorer.Rename(req.Item, req.NewItemPath)
		if err == nil {
			ApiSuccessResponse(c, "")
		} else {
			ApiErrorResponse(c, 400, err)
		}
	case "move":
		err := s.FileExplorer.Move(req.Items, req.NewPath)
		if err == nil {
			ApiSuccessResponse(c, "")
		} else {
			ApiErrorResponse(c, 400, err)
		}
	case "copy":
		err := s.FileExplorer.Copy(req.Items, req.NewPath, req.SingleFilename)
		if err == nil {
			ApiSuccessResponse(c, "")
		} else {
			ApiErrorResponse(c, 400, err)
		}
	case "remove":
		err := s.FileExplorer.Delete(req.Items)
		if err == nil {
			ApiSuccessResponse(c, "")
		} else {
			ApiErrorResponse(c, 400, err)
		}
	case "createFolder":
		err := s.FileExplorer.Mkdir(req.NewPath)
		if err == nil {
			ApiSuccessResponse(c, "")
		} else {
			ApiErrorResponse(c, 400, err)
		}
	case "changePermissions":
		err := s.FileExplorer.Chmod(req.Items, req.PermsCode, req.Recursive)
		if err == nil {
			ApiSuccessResponse(c, "")
		} else {
			ApiErrorResponse(c, 400, err)
		}
	case "getContent":
		rlt, err := s.FileExplorer.GetContent(req.Item)
		if err == nil {
			c.JSON(200, models.GetContentResp{Result: rlt})
		} else {
			ApiErrorResponse(c, 400, err)
		}
	case "edit":
		err := s.FileExplorer.Edit(req.Item, req.Content)
		if err == nil {
			ApiSuccessResponse(c, "")
		} else {
			ApiErrorResponse(c, 400, err)
		}
	case "compress":
		c.JSON(200, DEFAULT_API_ERROR_RESPONSE)
	case "extract":
		c.JSON(200, DEFAULT_API_ERROR_RESPONSE)
	default:
		c.JSON(200, DEFAULT_API_ERROR_RESPONSE)
	}
}

func IsApiPath(url string) bool {
	return strings.HasPrefix(url, "/api/") || strings.HasPrefix(url, "/bridges/php/handler.php")
}

func uploadHandler(c *macaron.Context, req *http.Request, s SessionInfo) {

	switch req.Method {
	case "POST":
		// Method when it fills memory buffer and rest save to temporary files on disk before move to destination
		// Not best way for handling large files
		// err := req.ParseMultipartForm(100000)
		// if err != nil {
		// 	c.JSON(200, models.GenericResp{
		// 		models.GenericRespBody{false, err.Error()},
		// 	})
		// }
		// m := req.MultipartForm
		// fmt.Println(m)
		// utils.Dump(m)
		// destination := m.Value["destination"][0]
		// files := m.File
		// fmt.Println(destination)
		// utils.Dump(files)
		// for i, v := range files {
		// 	fmt.Println(i)
		// utils.Dump(v)
		// }

		destination := ""

		reader, err := req.MultipartReader()
		if err != nil {
			c.JSON(200, models.GenericResp{
				models.GenericRespBody{false, err.Error()},
			})
		}
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}

			// fmt.Println(part.FormName())
			if part.FormName() == "destination" {
				buf := new(bytes.Buffer)
				buf.ReadFrom(part)
				destination = buf.String()
				fmt.Println(destination)
			}

			if part.FileName() == "" {
				continue
			}

			if len(destination) == 0 {
				continue
			}

			err = s.FileExplorer.UploadFile(destination, part)
			if err != nil {
				c.JSON(200, models.GenericResp{
					models.GenericRespBody{false, err.Error()},
				})
			}

			fmt.Println("filename=", part.FileName())
			// dst, err := os.Create(fmt.Sprintf("%s/%s", destination, part.FileName()))
			// defer dst.Close()
			// if err != nil {
			// 	c.JSON(200, models.GenericResp{
			// 		models.GenericRespBody{false, err.Error()},
			// 	})
			// }

			// if _, err := io.Copy(dst, part); err != nil {
			// 	c.JSON(200, models.GenericResp{
			// 		models.GenericRespBody{false, err.Error()},
			// 	})
			// }
		}
		ApiSuccessResponse(c, "")

	default:
		c.JSON(200, DEFAULT_API_ERROR_RESPONSE)
	}

}

func downloadHandler(c *macaron.Context, req *http.Request, s SessionInfo) {
	switch req.Method {
	case "GET":
		params := req.URL.Query()

		fileinfo, err := os.Stat(params.Get("path"))
		if err != nil {
			c.JSON(200, models.GenericResp{
				models.GenericRespBody{false, err.Error()},
			})
		}
		data, err := ioutil.ReadFile(params.Get("path"))
		if err != nil {
			c.JSON(200, models.GenericResp{
				models.GenericRespBody{false, err.Error()},
			})
		}

		c.Header().Set("content-disposition", fmt.Sprintf("attachment; filename=\"%s\"", fileinfo.Name()))
		http.ServeContent(c, req, fileinfo.Name(), time.Now(), bytes.NewReader(data))

	default:
		c.JSON(200, DEFAULT_API_ERROR_RESPONSE)
	}
	c.JSON(200, DEFAULT_API_ERROR_RESPONSE)
}

func Contexter() macaron.Handler {
	return func(c *macaron.Context, cache cache.Cache, session session.Store, f *session.Flash) {
		isSigned := false
		sessionInfo := SessionInfo{}
		uid := session.Get("uid")

		if uid == nil {
			uid = "janson"
		}

		if uid == nil {
			isSigned = false
		} else {
			sessionInfoObj := cache.Get(uid.(string))
			if sessionInfoObj == nil {
				sessionInfo.FileExplorer = fe.NewLocalFileExplorer(settings.Backend.Host)
				isSigned = true
				sessionInfo.User = "janson"
				c.Map(sessionInfo)
			} else {
				sessionInfo = sessionInfoObj.(SessionInfo)

				if settings.Backend.Type == "local" {
					sessionInfo.FileExplorer = fe.NewLocalFileExplorer(settings.Backend.Host)
					isSigned = true
					sessionInfo.User = "janson"
					c.Map(sessionInfo)
				} else if sessionInfo.User == "" || sessionInfo.Password == "" {
					isSigned = false
				} else {
					isSigned = true
					c.Data["User"] = sessionInfo.User
					c.Map(sessionInfo)
					if sessionInfo.FileExplorer == nil {
						fe, err := BackendConnect(sessionInfo.User, sessionInfo.Password)
						sessionInfo.FileExplorer = fe
						if err != nil {
							isSigned = false
							if IsApiPath(c.Req.URL.Path) {
								ApiErrorResponse(c, 500, err)
							} else {
								AuthError(c, f, err)
							}
						}
					}
				}
			}
		}

		if isSigned == false {
			if strings.HasPrefix(c.Req.URL.Path, "/login") {
				if c.Req.Method == "POST" {
					username := c.Query("username")
					password := c.Query("password")
					fe, err := BackendConnect(username, password)
					if err != nil {
						AuthError(c, f, err)
					} else {
						uid := username // TODO: ??
						sessionInfo = SessionInfo{username, password, fe, uid}
						cache.Put(uid, sessionInfo, 100000000000)
						session.Set("uid", uid)
						c.Data["User"] = sessionInfo.User
						c.Map(sessionInfo)
						c.Redirect("/")
					}
				}
			} else {
				c.Redirect("/login")
			}
		} else {
			if strings.HasPrefix(c.Req.URL.Path, "/logout") {
				sessionInfo.FileExplorer.Close()
				session.Delete("uid")
				cache.Delete(uid.(string))
				c.SetCookie("MacaronSession", "")
				c.Redirect("/login")
			}
		}
	}
}

func BackendConnect(username string, password string) (fe.FileExplorer, error) {
	fe := fe.NewSSHFileExplorer(settings.Backend.Host, username, password)
	err := fe.Init()
	if err == nil {
		return fe, nil
	}
	log.Println(err)
	return nil, err
}

func ApiErrorResponse(c *macaron.Context, code int, obj interface{}) {
	var message string
	if err, ok := obj.(error); ok {
		message = err.Error()
	} else {
		message = obj.(string)
	}
	c.JSON(code, models.GenericResp{models.GenericRespBody{false, message}})
}

func ApiSuccessResponse(c *macaron.Context, message string) {
	c.JSON(200, models.GenericResp{models.GenericRespBody{true, message}})
}

func AuthError(c *macaron.Context, f *session.Flash, err error) {
	f.Set("ErrorMsg", err.Error())
	c.Data["Flash"] = f
	c.Data["ErrorMsg"] = err.Error()
	c.Redirect("/login")
}
