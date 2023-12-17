# LS-PaintBoard API

采用 Golang 与协程技术，token 利用率达到了 98% 左右。

## 使用方法

```
go build jeefy/main
```

生成可执行文件，如果在 `Linux` 下需要制定生成的文件名：

```
go build -o xxx jeefy/main
```

## 命令行模式

直接运行进入命令行模式，基本不用。

## 配置模式

单图片模式采用 `config.txt` 文件，具体格式：

```
图片地址.png [ignore/not]
X Y
n
uid paste
...
```

其中 `[ignore/not]` 表示如果指定 `ignore` 那么将忽略纯白色（`#FFFFFF`）的色块。

`X, Y` 是图片要绘制的地址

`n` 表示 `token` 的数量，接下来 `n` 行先是 `uid`，然后跟 `paste`

获取到的 `token` 会缓存在 `_api.txt` 中，运行开始自动读取。

如果需要忽略命令行模式，请运行 `xxx start`

## 多任务模式

多图片有多任务模式，但是是一个一个图片画。

将 `config.txt` 多复制几份，每张图片一份。

在 `tasks.txt` 中保存如下内容：

- 第一行一个数表示图片的数量
- 接下来每行一个 `config` 文件地址

运行 `xxx tasks tasks.txt`，其中 `xxx` 是可执行文件，`tasks.txt` 可以替换为保存任务配置的地址。
