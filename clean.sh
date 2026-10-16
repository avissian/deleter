#!/usr/bin/env bash
set -e

> out.txt
go run . /home/dps/docker/webtlo/webtlo-data/storage/config.ini

echo "К удалению:"
wc -l out.txt
head -5 out.txt
echo
echo "Удалять? y/N"
read -r ans
[ "$ans" = "y" ] || { echo "Отмена"; exit 1; }
echo Delete...
time xargs -d "\n" -I{} -a out.txt rm "{}"
df -h | grep -E "Avail|/mnt/"
