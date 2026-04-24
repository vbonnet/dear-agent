// Sample TypeScript code with intentional issues for review

export class UserService {
  // ISSUE: Hardcoded credentials
  private apiKey = 'sk-1234567890abcdef';

  async fetchUserData(userId: string) {
    // ISSUE: No input validation
    const url = `https://api.example.com/users/${userId}`;

    // ISSUE: No error handling
    const response = await fetch(url, {
      headers: {
        'Authorization': `Bearer ${this.apiKey}`
      }
    });

    // ISSUE: No response validation
    const data = await response.json();
    return data;
  }

  // ISSUE: Inefficient implementation
  processUsers(users: any[]) {
    let results = [];
    for (let i = 0; i < users.length; i++) {
      for (let j = 0; j < users.length; j++) {
        if (users[i].id === users[j].friendId) {
          results.push(users[i]);
        }
      }
    }
    return results;
  }

  // ISSUE: Type safety concerns
  saveUserPreferences(userId: any, preferences: any) {
    // ISSUE: Direct database query without sanitization
    const query = `UPDATE users SET preferences = '${JSON.stringify(preferences)}' WHERE id = ${userId}`;
    // Simulated database call
    console.log('Executing:', query);
  }
}
