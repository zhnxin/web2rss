package main

const htmlTmpl = `<!DOCTYPE html>
<html lang="en">

<head>
    <meta charset="UTF-8">
    <meta http-equiv="X-UA-Compatible" content="IE=edge">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>schedule</title>
    <style>
        table {
            font-family: arial, sans-serif;
            border-collapse: collapse;
            width: 100%;
        }

        td,th {
            border: 1px solid #dddddd;
            text-align: left;
            padding: 8px;
        }

        tr:nth-child(even) {
            background-color: #dddddd;
        }
        .loading {
            width: 15px;
            height: 15px;
            border: 2px solid #5b5656ff;
            border-top-color: transparent;
            border-radius: 100%;

            animation: circle infinite 0.75s linear;
        }

        @keyframes circle {
            0% {
                transform: rotate(0);
            }
            100% {
                transform: rotate(360deg);
            }
        }
        .completed {
            width: 15px;
            height: 15px;
            border: 2px solid #4CAF50;
            border-radius: 100%;
        }
        
    </style>
</head>

<body>
    <table>
        <thead>
            <tr>
                <th>频道</th>
                <th>状态</th>
                <th>time</th>
            </tr>
        </thead>
        <tbody>
            {{range .}}<tr>
                <td> <a href='/html/{{.Item}}' >{{.Item}}</a></td>
                <td>{{if .Update}}<div class="loading"></div>{{else}}<div class="completed"></div>{{end}}</td>
                <td>{{.T.Format "2006-01-02T15:04:05Z07:00"}}</td>
            </tr>
            {{end}}
        </tbody>
    </table>
</body>

</html>`
const channelTableHtml = `
<!DOCTYPE html>
<html lang="en">

<head>
    <meta charset="UTF-8">
    <meta http-equiv="X-UA-Compatible" content="IE=edge">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>channel</title>
</head>

<body>
    <ul>
        {{range .}}
            <li class="column">
                <a href="/html/{{.Channel}}/{{.Mk}}">{{.Title}}</a>
                <span>{{.PubDate}}</span>
            </li>
        {{end}}
    </ul>
</body>

</html>
`
const itemDetailHtml = `
<!DOCTYPE html>
<html lang="en">

<head>
    <meta charset="UTF-8">
    <meta http-equiv="X-UA-Compatible" content="IE=edge">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}}</title>
</head>

<body>
    <h3>{{.Title}}</h3>
    {{if .Thumb }}<img src="{{.Thumb }}" />{{end}}
    {{.Description.Content}}
</body>

</html>
`
const itemNotFoundPage=`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Document</title>
</head>
<body>
    <a href="/html/%s">%s</a>
</body>
</html>`