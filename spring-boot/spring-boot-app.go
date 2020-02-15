/*
 * Copyright 2012-2019 the original author or authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// 开箱即用的 Go-Spring 程序启动框架。
package SpringBoot

import (
	"os"
	"strings"

	"github.com/go-spring/go-spring-parent/spring-logger"
	"github.com/go-spring/go-spring/spring-core"
)

const (
	DefaultConfigLocation = "config/" // 默认的配置文件路径

	SpringAccess   = "spring.access" // "all" 为允许注入私有字段
	SPRING_ACCESS  = "SPRING_ACCESS"
	SpringProfile  = "spring.profile" // 运行环境
	SPRING_PROFILE = "SPRING_PROFILE"
	SpringStrict   = "spring.strict" // 严格模式，"true" 必须使用 AsInterface() 导出接口
	SPRING_STRICT  = "SPRING_STRICT"
)

// CommandLineRunner 命令行启动器接口
type CommandLineRunner interface {
	Run(ctx ApplicationContext)
}

// ApplicationEvent 应用运行过程中的事件
type ApplicationEvent interface {
	OnStartApplication(ctx ApplicationContext) // 应用启动的事件
	OnStopApplication(ctx ApplicationContext)  // 应用停止的事件
}

// application SpringBoot 应用
type application struct {
	appCtx      ApplicationContext // 应用上下文
	cfgLocation []string           // 配置文件目录
	configReady func()             // 配置文件已就绪
}

// newApplication application 的构造函数
func newApplication(appCtx ApplicationContext, cfgLocation ...string) *application {
	if len(cfgLocation) == 0 { // 没有的话用默认的配置文件路径
		cfgLocation = append(cfgLocation, DefaultConfigLocation)
	}
	return &application{
		appCtx:      appCtx,
		cfgLocation: cfgLocation,
	}
}

// Start 启动 SpringBoot 应用
func (app *application) Start() {
	// 配置项加载顺序优先级:
	// 1.在此之前的代码设置
	// 2.命令行参数(暂未支持)
	// 3.系统环境变量
	// 4.配置文件

	// 加载系统环境变量
	app.loadSystemEnv()

	// 加载配置文件
	app.loadConfigFiles()

	// 准备上下文环境
	app.prepare()

	// 注册 ApplicationContext
	app.appCtx.RegisterBean(app.appCtx).AsInterface(
		(*ApplicationContext)(nil), (*SpringCore.SpringContext)(nil),
	)

	// 依赖注入、属性绑定、Bean 初始化
	app.appCtx.AutoWireBeans()

	// 执行命令行启动器
	var runners []CommandLineRunner
	app.appCtx.CollectBeans(&runners)

	for _, r := range runners {
		r.Run(app.appCtx)
	}

	// 通知应用启动事件
	var eventBeans []ApplicationEvent
	app.appCtx.CollectBeans(&eventBeans)

	for _, bean := range eventBeans {
		bean.OnStartApplication(app.appCtx)
	}

	SpringLogger.Info("spring boot started")
}

func (app *application) loadSystemEnv() {
	SpringLogger.Debugf(">>> load system env")
	for _, env := range os.Environ() {
		if i := strings.Index(env, "="); i > 0 {
			k, v := env[0:i], env[i+1:]
			k = strings.ToLower(k)
			SpringLogger.Tracef("%s=%v", k, v)
			app.appCtx.SetProperty(k, v)
		}
	}
}

func (app *application) loadConfigFiles() {

	// 加载默认的应用配置文件，如 application.properties
	app.loadProfileConfig("")

	// 加载用户设置的配置文件，如 application-test.properties
	keys := []string{SpringProfile, SPRING_PROFILE}
	if profile := SpringCore.GetStringProperty(app.appCtx, keys...); profile != "" {
		app.loadProfileConfig(strings.ToLower(profile))
	}
}

func (app *application) loadProfileConfig(profile string) {
	for _, configLocation := range app.cfgLocation {

		var result map[string]interface{}

		if ss := strings.Split(configLocation, ":"); len(ss) == 1 {
			result = NewDefaultPropertySource(ss[0]).Load(profile)
		} else {
			switch ss[0] {
			case "k8s":
				result = NewConfigMapPropertySource(ss[1]).Load(profile)
			}
		}

		for k, v := range result {
			app.appCtx.SetProperty(k, v)
		}
	}
}

// prepare 准备上下文环境
func (app *application) prepare() {

	// 设置运行环境
	keys := []string{SpringProfile, SPRING_PROFILE}
	if profile := SpringCore.GetStringProperty(app.appCtx, keys...); profile != "" {
		app.appCtx.SetProfile(strings.ToLower(profile))
	}

	// 设置是否允许注入私有字段
	keys = []string{SpringAccess, SPRING_ACCESS}
	if access := SpringCore.GetStringProperty(app.appCtx, keys...); access != "" {
		app.appCtx.SetAllAccess(strings.ToLower(access) == "all")
	}

	// 配置文件已就绪
	if app.configReady != nil {
		app.configReady()
	}
}

// ShutDown 停止 SpringBoot 应用
func (app *application) ShutDown() {

	// 通知 Bean 销毁
	app.appCtx.Close()

	// 通知应用停止事件
	var eventBeans []ApplicationEvent
	app.appCtx.CollectBeans(&eventBeans)

	for _, bean := range eventBeans {
		bean.OnStopApplication(app.appCtx)
	}

	// 等待所有 goroutine 退出
	app.appCtx.Wait()

	SpringLogger.Info("spring boot exited")
}
