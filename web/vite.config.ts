import {defineConfig} from 'vite';
import react from '@vitejs/plugin-react';
export default defineConfig({plugins:[react()],server:{host:'127.0.0.1',port:5173,strictPort:true,proxy:{'/api':{target:process.env.MOVEMENT_API_URL || 'http://127.0.0.1:8484',changeOrigin:false}}},build:{outDir:'../internal/web/dist',emptyOutDir:true,target:['safari17','chrome120','firefox121']}});
