package main

import (
	"errors"
	"fmt"
	"github.com/baa-middleware/session"
	"github.com/syyongx/php2go"
	"gopkg.in/baa.v1"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"runtime"
)

func isUserLoggedIn(c *baa.Context) bool {
	username := getUsername(c)
	if username == nil {
		return false
	}
	return true
}

func getUsername(c *baa.Context) interface{} {
	// get the session handler
	sessionObj := c.Get("session").(*session.Session)
	return sessionObj.Get("username")
}

func letUserLogIn(c *baa.Context, username string) {
	// get the session handler
	sessionObj := c.Get("session").(*session.Session)
	sessionObj.Set("username", username)

	//尝试创建用户主目录
	php2go.Mkdir(getUserMainDirectory(c), 0777)

	execCommand(c, getGitInitCmd(getUserMainDirectory(c)))
}

func letUserLogOut(c *baa.Context) {
	// get the session handler
	sessionObj := c.Get("session").(*session.Session)
	sessionObj.Delete("username")
}

//with suffix / or \
func getCurrentPath() (string, error) {
	file, err := exec.LookPath(os.Args[0])
	if err != nil {
		return "", err
	}
	path, err := filepath.Abs(file)
	if err != nil {
		return "", err
	}
	i := strings.LastIndex(path, string(os.PathSeparator))
	if i < 0 {
		return "", errors.New(`can't find "/" or "\"`)
	}
	return string(path[0 : i+1]), nil
}

func isFileExists(path string) (bool, error) {
	fileInfoData, err := os.Stat(path)
	if fileInfoData == nil {
		return false, nil
	}
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, err
}

func fileGetContent(fullFilePath string) string {
	content, _ := php2go.FileGetContents(fullFilePath)
	return content
}

//without suffix \ or /
func getUserMainDirectory(c *baa.Context) string {
	// get the session handler
	sessionObj := c.Get("session").(*session.Session)
	username := sessionObj.Get("username")
	path, _ := getCurrentPath()
	return path + "notes" + string(os.PathSeparator) + username.(string)
}

func getPassword(username string) interface{} {
	path, _ := getCurrentPath()
	fullFilePath := path + "password" + string(os.PathSeparator) + username
	isExists, _ := isFileExists(fullFilePath)
	if isExists {
		password := fileGetContent(fullFilePath)
		return password
	}
	return nil
}

// dir is with suffix / or \
func getFilesUnderDir(dir string) []string {
	files, _ := filepath.Glob(dir + "*")
	return files
}

type FileInfoData struct {
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
}

type JsonError struct {
	Message string `json:"message"`
}

func getGitInitCmd(dir string) string {
	dir = php2go.Rtrim(dir, string(os.PathSeparator))
	dir = dir + string(os.PathSeparator)
	return fmt.Sprintf("git init %s", dir)
}

func getGitAddAllCmd(dir string) string {
	dirnosuffix := php2go.Rtrim(dir, string(os.PathSeparator))
	dir = dirnosuffix + string(os.PathSeparator)
	return fmt.Sprintf("git --git-dir=\"%s.git\" --work-tree=\"%s\" add -A", dir, dirnosuffix)
}

func getGitCommitCmd(dir string, filePath string) string {
	dirnosuffix := php2go.Rtrim(dir, string(os.PathSeparator))
	dir = dirnosuffix + string(os.PathSeparator)
	return fmt.Sprintf("git --git-dir=\"%s.git\" --work-tree=\"%s\" commit -m \"update file %s\"", dir, dirnosuffix, filePath)
}

//func getGitStatusCmd(dir string) string {
//	dirnosuffix := php2go.Rtrim(dir, "/")
//	dir = dirnosuffix + "/"
//	return fmt.Sprintf("git --git-dir=\"%s.git\" --work-tree=\"%s\" status", dir)
//}

func execCommand(c *baa.Context, cmd string) {
	var err error
	var newCmd string
	var shellFilePath string

	shellFilePath = getUserMainDirectory(c) + string(os.PathSeparator) + "git_shell.sh"
	newCmd = "#!/bin/bash\n\n" + cmd

	if runtime.GOOS == "windows" {
		shellFilePath = getUserMainDirectory(c) + string(os.PathSeparator) + "git_shell.bat"
		newCmd = cmd
	}

	php2go.FilePutContents(shellFilePath, newCmd, 0777)
	err = exec.Command(shellFilePath).Run()
	
	if err != nil {
		fmt.Println("error occurred")
		fmt.Printf("%s", err)
	}
}

func main() {
	// new app
	app := baa.New()

	// use session middleware
	memoryOptions := session.MemoryOptions{}
	memoryOptions.BytesLimit = 1024 * 1024 * 10 // 每个session缓存分配最多10M

	app.Use(session.Middleware(session.Options{
		Name: "GSESSION",
		Provider: &session.ProviderOptions{
			Adapter: "memory",
			Config:  memoryOptions,
		},
	}))

	// main page
	app.Get("/", func(c *baa.Context) {
		isLoggedIn := isUserLoggedIn(c)
		if isLoggedIn {
			c.Set("username", getUsername(c))
			c.HTML(200, "template/index.html")
		} else {
			c.Redirect(302, "/login")
			return
		}
	})

	//获取该用户目录下的一级笔记
	// 格式 `/ls?dir=`
	app.Get("/ls", func(c *baa.Context) {
		isLoggedIn := isUserLoggedIn(c)
		if isLoggedIn {
			rootPath := getUserMainDirectory(c)
			dir := "/"

			if len(c.Query("dir")) > 0 {
				dir = c.Query("dir")
				dir = strings.Replace(dir, ".", "", -1)
				dir = strings.Replace(dir, "\\", "", -1)
				dir = php2go.Trim(dir, "/")
				dir = "/" + dir + "/"
			}
			if runtime.GOOS == "windows" {
				dir = strings.Replace(dir, "/", string(os.PathSeparator), -1)
			}
			//fullDir is with suffix / or \
			fullDir := rootPath + dir

			files := getFilesUnderDir(fullDir)
			var fileInfo os.FileInfo
			var fileInfoDataName string
			var pos int
			var fileInfoDataList = make([]FileInfoData, len(files))

			for i, fileFullPath := range files {
				fileInfo, _ = os.Stat(fileFullPath)
				pos = len(rootPath)
				fileInfoDataName = php2go.Substr(fileFullPath, uint(pos), -1)
				if runtime.GOOS == "windows" {
					fileInfoDataName = strings.Replace(fileInfoDataName, string(os.PathSeparator), "/", -1)
				}
				fileInfoDataList[i] = FileInfoData{Path: fileInfoDataName, IsDir: fileInfo.IsDir()}
			}

			c.JSON(200, fileInfoDataList)
		} else {
			c.JSON(401, JsonError{Message: "please login"})
			return
		}
	})

	//获取某个笔记具体内容
	// 格式 `/get?path=`
	app.Get("/get", func(c *baa.Context) {
		isLoggedIn := isUserLoggedIn(c)
		if isLoggedIn {
			rootPath := getUserMainDirectory(c)
			var path string

			if len(c.Query("path")) == 0 {
				c.JSON(410, JsonError{Message: "file not exists"})
				return
			}
			path = c.Query("path")
			path = strings.Replace(path, "..", "", -1)
			path = strings.Replace(path, "\\", "", -1)
			path = php2go.Trim(path, "/")

			if runtime.GOOS == "windows" {
				path = strings.Replace(path, "/", string(os.PathSeparator), -1)
			}

			fullPath := rootPath + string(os.PathSeparator) + path
			isExists, _ := isFileExists(fullPath)
			if !isExists {
				c.JSON(410, JsonError{Message: "file not exists"})
				return
			}

			c.JSON(200, fileGetContent(fullPath))
		} else {
			c.JSON(401, JsonError{Message: "please login"})
			return
		}
	})

	//保存笔记且使用git版本控制
	// 格式 `/put?path=`
	app.Post("/put", func(c *baa.Context) {
		isLoggedIn := isUserLoggedIn(c)
		if isLoggedIn {
			rootPath := getUserMainDirectory(c)
			var path string

			if len(c.Query("path")) == 0 {
				c.JSON(410, JsonError{Message: "file not exists"})
				return
			}

			content := c.Query("content")

			path = c.Query("path")
			path = strings.Replace(path, "..", "", -1)
			path = strings.Replace(path, "\\", "", -1)
			path = php2go.Trim(path, "/")

			if runtime.GOOS == "windows" {
				path = strings.Replace(path, "/", string(os.PathSeparator), -1)
			}

			fullPath := rootPath + string(os.PathSeparator) + path
			fileInfo, _ := os.Stat(fullPath)
			if fileInfo != nil {
				if fileInfo.IsDir() {
					c.JSON(410, JsonError{Message: "不能写入文件：" + path})
					return
				}
			}

			isExists, _ := isFileExists(fullPath)
			if !isExists {
				pos := php2go.Strpos(fullPath, filepath.Base(fullPath), 0)
				dirname := php2go.Substr(fullPath, 0, pos)
				err := php2go.Mkdir(dirname, 0777)
				fmt.Println(err)
				if err != nil {
					fileInfo, _ = os.Stat(dirname)
					if !fileInfo.IsDir() {
						c.JSON(410, JsonError{Message: "不能写入文件：" + path})
						return
					}
				}
			}
			php2go.FilePutContents(fullPath, content, 0777)

			c.JSON(200, fileGetContent(fullPath))

			execCommand(c, getGitAddAllCmd(getUserMainDirectory(c)))
			execCommand(c, getGitCommitCmd(getUserMainDirectory(c), filepath.Base(fullPath)))
		} else {
			c.JSON(401, JsonError{Message: "please login"})
			return
		}
	})

	//登录登出

	app.Get("/login", func(c *baa.Context) {
		sessionObj := c.Get("session").(*session.Session)
		loginError := sessionObj.Get("loginError")
		sessionObj.Delete("loginError")

		c.Set("loginError", loginError)
		c.HTML(200, "template/login.html")
	})

	app.Post("/login", func(c *baa.Context) {
		username := c.Query("username")
		password := c.Query("password")

		if (len(password) > 0)&&(password == getPassword(username)) {
			letUserLogIn(c, username)
			c.Redirect(302, "/")
			return
		} else {
			sessionObj := c.Get("session").(*session.Session)
			sessionObj.Set("loginError", "密码错误")
			c.Redirect(302, "/login")
			return
		}
	})

	app.Get("/logout", func(c *baa.Context) {
		letUserLogOut(c)
		c.Redirect(302, "/")
		return
	})

	currentPath, _ := getCurrentPath()
	app.Static("/assets", currentPath + "assets", true, func(c *baa.Context) {
		// 你可以对输出的结果干点啥的
	})

	// run app
	app.Run(":1323")
}
