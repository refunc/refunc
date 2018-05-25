#!/usr/bin/env sh

# 确保脚本抛出遇到的错误
set -e

vuepress build

# 进入生成的文件夹
cd .vuepress/dist

# 如果是发布到自定义域名
echo 'refunc.io' > CNAME

git init
git add -A
git commit -m "[*] Deploy"

git push -f git@github.com:refunc/refunc.github.io.git master

cd -