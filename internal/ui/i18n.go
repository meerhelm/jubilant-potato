package ui

import (
	"fmt"
	"os"
	"strings"
)

var lang = "en"

func setLanguage(l string) {
	if l == "" {
		l = strings.ToLower(os.Getenv("LANG"))
	}
	if strings.HasPrefix(l, "ru") {
		lang = "ru"
	}
}

// T returns the translation of an English UI string.
func T(s string, args ...any) string {
	if lang == "ru" {
		if r, ok := ru[s]; ok {
			s = r
		}
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}

var ru = map[string]string{
	"Sources":                "Источники",
	"Downloads":              "Загрузки",
	"Downloads (%d active)":  "Загрузки (активно: %d)",
	"Open":                   "Открыть",
	"Back":                   "Назад",
	"Quit":                   "Выход",
	"Download":               "Скачать",
	"Search":                 "Поиск",
	"Letter":                 "Буква",
	"Cancel":                 "Отмена",
	"Retry":                  "Повтор",
	"Clear done":             "Убрать готовые",
	"Loading…":               "Получаю список…",
	"Error: %s":              "Ошибка: %s",
	"No sources configured.": "Источники не настроены.",
	"Add them to config.json next to the app": "Добавьте их в config.json рядом с приложением",
	"Nothing here":                      "Здесь пусто",
	"No downloads yet":                  "Загрузок пока нет",
	"Queued: %s":                        "В очереди: %s",
	"Already installed":                 "Уже установлено",
	"Already in queue":                  "Уже в очереди",
	"%d games":                          "Игр: %d",
	"Free: %s":                          "Свободно: %s",
	"Filter: %s":                        "Фильтр: %s",
	"queued":                            "в очереди",
	"downloading":                       "загрузка",
	"extracting":                        "распаковка",
	"done":                              "готово",
	"failed":                            "ошибка",
	"canceled":                          "отменено",
	"Space":                             "Пробел",
	"Delete":                            "Стереть",
	"Done":                              "Готово",
	"Type":                              "Ввод",
	"Clear":                             "Очистить",
	"Search archive.org":                "Поиск на archive.org",
	"search collections":                "поиск коллекций",
	"Add":                               "Добавить",
	"Looking at the files…":             "Смотрю файлы…",
	"Which system are these games for?": "Для какой системы эти игры?",
	"files: %d":                         "файлов: %d",
	"Added to %s":                       "Добавлено в %s",
	"Remove":                            "Удалить",
	"Remove %s?":                        "Удалить «%s»?",
	"Yes":                               "Да",
	"No":                                "Нет",
	"Select":                            "Выбрать",
	"Add source":                        "Добавить источник",
	"game library server":               "сервер библиотеки игр",
	"Network folder (SMB)":              "Сетевая папка (SMB)",
	"NAS, Windows, macOS":               "NAS, Windows, macOS",
	"Web folder (HTTP)":                 "Веб-папка (HTTP)",
	"directory listing":                 "список файлов",
	"Folder address":                    "Адрес папки",
	"Invalid address":                   "Неверный адрес",
	"Search again":                      "Искать снова",
	"Searching %s…":                     "Ищу в сети %s…",
	"Found: %d":                         "Найдено: %d",
	"Enter address manually":            "Ввести адрес вручную",
	"Find RomM":                         "Поиск RomM",
	"RomM address":                      "Адрес RomM",
	"Find network folders":              "Поиск сетевых папок",
	"Server address":                    "Адрес сервера",
	"User on %s":                        "Пользователь на %s",
	"Password for %s":                   "Пароль для %s",
	"Connecting to %s…":                 "Подключаюсь к %s…",
	"Login required":                    "Нужен вход",
	"Press A to sign in":                "Нажмите A, чтобы войти",
	"Use this folder":                   "Использовать эту папку",
	"systems: %d":                       "систем: %d",
	"Button setup":                      "Настройка кнопок",
	"Press %s":                          "Нажмите %s",
	"Step %d of %d":                     "Шаг %d из %d",
	"No such button? Wait %d s to skip": "Нет такой кнопки? Пропуск через %d с",
	"Esc on a keyboard cancels":         "Esc на клавиатуре — отмена",
	"Buttons saved":                     "Кнопки сохранены",
	"any button":                        "любая",
	"ROM downloader":                    "Загрузчик ROM-ов",
	"Versions":                          "Версии",
	"All":                               "Все",
	"Releases":                          "Релизы",
	"all versions":                      "все версии",
	"preferred":                         "основная",
	"Showing betas, demos and hacks":    "Показаны беты, демо и хаки",
	"Showing releases only":             "Только официальные релизы",
	"Connect to %s":                     "Подключение: %s",
	"Connected":                         "Подключено",
	"Scan with your phone":              "Отсканируйте телефоном",
	"or open in a browser:":             "или откройте в браузере:",
	"and enter the code:":               "и введите код:",
	"Waiting for approval…":             "Жду подтверждения…",
	"Code expires in %s":                "Код действует ещё %s",
	"The code has expired":              "Срок действия кода истёк",
	"Pairing was declined":              "Подключение отклонено",
	"Press A to try again":              "Нажмите A, чтобы попробовать снова",
}
