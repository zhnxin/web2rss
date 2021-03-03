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

        td,
        th {
            border: 1px solid #dddddd;
            text-align: left;
            padding: 8px;
        }

        tr:nth-child(even) {
            background-color: #dddddd;
        }
    </style>
</head>

<body>
    <table>
        <thead>
            <tr>
                <th>item</th>
                <th>time</th>
            </tr>
        </thead>
        <tbody>
            {{range .}}<tr>
                <td>{{.Item}}</td>
                <td>{{.T.Format "2006-01-02T15:04:05Z07:00"}}</td>
            </tr>
            {{end}}
        </tbody>
    </table>
</body>

</html>`
