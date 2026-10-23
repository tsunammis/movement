import {defineConfig} from '@playwright/test';
export default defineConfig({testDir:'./tests',timeout:60000,workers:1,fullyParallel:false,use:{actionTimeout:10000,channel:'chrome',headless:true,viewport:{width:1360,height:900},trace:'retain-on-failure'},projects:[{name:'packaged'},{name:'vite'}]});
