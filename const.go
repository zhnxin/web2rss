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
    <button onclick="fetchData()">刷新</button>
    <button onclick="updateChannel('')">更新</button>
    <table>
        <thead>
            <tr>
                <th>频道</th>
                <th>状态</th>
                <th>time</th>
                <th>操作</th>
            </tr>
        </thead>
        <tbody id="content">
            {{range .}}<tr>
                <td> <a href='/html/{{.Item}}' >{{.Item}}</a></td>
                <td>{{if .Update}}<div class="loading"></div>{{else}}<div class="completed"></div>{{end}}</td>
                <td>{{.T.Format "2006-01-02T15:04:05Z07:00"}}</td>
                <td><button onclick="updateChannel('{{.Item}}')">更新</button></td>
            </tr>
            {{end}}
        </tbody>
    </table>
    <script src="https://unpkg.com/axios/dist/axios.min.js"></script>
    <script>
        function fetchData() {
            axios.get('web2rss', {
                headers: {
                    'Accept': 'application/json'
                }
            }).then(function (response) {
                const tbody = document.getElementById('content')
                tbody.innerHTML = ''
                console.log(response.data);
                (response.data.data || []).forEach(function (item) {
                    const tr = document.createElement('tr')

                    const tdItem = document.createElement('td')
                    const a = document.createElement('a')
                    a.href = '/html/' + item.item
                    a.textContent = item.item
                    tdItem.appendChild(a)
                    tr.appendChild(tdItem)

                    const tdStatus = document.createElement('td')
                    if (item.is_update) {
                        const divLoading = document.createElement('div')
                        divLoading.className = 'loading'
                        tdStatus.appendChild(divLoading)
                    } else {
                        const divCompleted = document.createElement('div')
                        divCompleted.className = 'completed'
                        tdStatus.appendChild(divCompleted)
                    }
                    tr.appendChild(tdStatus)

                    const tdTime = document.createElement('td')
                    tdTime.textContent = new Date(item.t).toISOString()
                    tr.appendChild(tdTime)

                    const tdAction = document.createElement('td')
                    const button = document.createElement('button')
                    button.textContent = '更新'
                    button.onclick = function () {
                        updateChannel(item.item)
                    }
                    tdAction.appendChild(button)
                    tr.appendChild(tdAction)

                    tbody.appendChild(tr)
                })
            }).catch(function (error) {
                console.error('Error fetching data:', error);
            });
        }
        function updateChannel(channel) {
            axios.put('web2rss/signal', {
                cmd: "update",args:channel
            }).then(function (response) {
                alert('已触发更新:' + channel)
                fetchData()
            }).catch(function (error) {
                alert('更新失败:' + error)
            });
        }
    </script>
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
    <a href='html' >返回</a>
    <ul id="content">
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