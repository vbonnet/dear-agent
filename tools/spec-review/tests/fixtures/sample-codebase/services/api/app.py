from flask import Flask, request
import psycopg2

app = Flask(__name__)

@app.route('/users')
def get_users():
    conn = psycopg2.connect("dbname=users")
    # Query users
    return {"users": []}

if __name__ == '__main__':
    app.run(port=5000)
