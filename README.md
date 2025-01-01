# tiny_url
轻量级短url转换，支持订阅转换sub-web

基于boltdb存储

## Docker使用
使用 下载代码解压后
```
docker build -t tinyurl:latest .
```
修改.env中SHORT_URL_BASE为短链接主域名

```
docker run -p 8000:8000 --env-file ./.env --restart always --name tinyurl tinyurl:latest
```
