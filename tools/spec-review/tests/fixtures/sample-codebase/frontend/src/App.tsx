import React from 'react';
import axios from 'axios';

export const App: React.FC = () => {
    const [users, setUsers] = React.useState([]);

    React.useEffect(() => {
        axios.get('http://localhost:5000/users')
            .then(res => setUsers(res.data.users));
    }, []);

    return <div>Users: {users.length}</div>;
};
