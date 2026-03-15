# Maximus CLI

Maximus CLI is a command-line interface for managing toil. In this context, toil is considered as repetitive tasks that can be automated and that the current cli tools are not able to handle in a user friendly way or require many steps to be performed.

`maximux-cli` is a command-line tool built with Go that makes using other terminal tools easier. It uses a simple text-based interface to help you run commands without having to remember exact flags or options.

Currently, it focuses on making Homebrew (`brew`) easier to use, but it is built to support other tools in the future. It also uses a local SQLite database to remember your settings and history between uses.

## Features
* **Simple Interface:** Easy-to-use menus built with Bubble Tea.
* **Homebrew Support:** Run and manage `brew` commands quickly.
* **Saves Your Setup:** Uses SQLite to remember your choices.