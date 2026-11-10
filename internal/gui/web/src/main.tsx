import React from 'react';
import ReactDOM from 'react-dom/client';
import App from './App';
import './app.css';

// Apply the default light theme before React paints to avoid a dark flash.
if (window.localStorage.getItem('agentbox-theme') !== 'dark') {
  document.documentElement.classList.add('theme-light');
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
